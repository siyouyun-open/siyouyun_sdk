package taskcenter

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type clientStruct struct {
	nc             *nats.Conn
	taskCenterImpl TaskCenterInterface
	taskConsumers  *sync.Map
	taskTypeMap    *sync.Map
	taskHandlers   *sync.Map
}

var Client *clientStruct

func safeGo(task func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[PANIC] Recovered from panic: %v\nStack trace:\n%s", r, debug.Stack())
			}
		}()
		task()
	}()
}

func Init(nc *nats.Conn, taskHandler TaskCenterInterface) {
	Client = &clientStruct{
		nc:             nc,
		taskCenterImpl: taskHandler,
		taskConsumers:  &sync.Map{},
		taskTypeMap:    &sync.Map{},
		taskHandlers:   &sync.Map{},
	}
	taskHandler.HandleInterruptTask()
	Client.subscribeTaskConsumer()
}

func (c *clientStruct) GetTaskCenter() TaskCenterInterface {
	return c.taskCenterImpl
}

func (c *clientStruct) RegisterTaskType(processor Processor) {
	taskTypeDO := processor.Config()
	if taskTypeDO == nil || taskTypeDO.valid() != nil {
		return
	}
	var flag AbilityFlag
	if _, ok := processor.(RunProcessor); ok {
		flag |= HandlerProcessing
	}
	if _, ok := processor.(ResumeProcessor); ok {
		flag |= HandlerResume
	}
	if _, ok := processor.(PauseProcessor); ok {
		flag |= HandlerPause
	}
	if _, ok := processor.(CancelProcessor); ok {
		flag |= HandlerCancel
	}
	if _, ok := processor.(ForceRemoveProcessor); ok {
		flag |= HandlerForceRemove
	}
	if flag == 0 {
		return
	}
	taskTypeDO.AbilityFlag = int(flag)
	c.taskTypeMap.Store(taskTypeDO.TaskType, taskTypeDO)
	c.taskHandlers.Store(taskTypeDO.TaskType, processor)
	safeGo(func() { _ = c.taskCenterImpl.SaveTaskType(taskTypeDO) })
	c.triggerTaskConsumer(taskTypeDO.TaskType)
}

func (c *clientStruct) GetTaskType(taskType string) (*TaskTypeDO, error) {
	taskTypeDO, ok := c.taskTypeMap.Load(taskType)
	if !ok {
		return nil, errors.New("task type not exist")
	}
	return taskTypeDO.(*TaskTypeDO), nil
}

func (c *clientStruct) GetTaskProcessor(taskType string) (Processor, error) {
	processor, ok := c.taskHandlers.Load(taskType)
	if !ok {
		return nil, errors.New("task type not exist")
	}
	return processor.(Processor), nil
}

func (c *clientStruct) RequestTask(task *TaskDO) error {
	if task == nil {
		return errors.New("task is null")
	}
	if err := task.valid(); err != nil {
		return err
	}

	if task.UUID == "" {
		task.UUID = uuid.NewString()
	}
	task.Status = TaskStatusWaiting
	task.StartAt = 0
	task.EndAt = 0
	task.handler = c.taskCenterImpl

	if err := c.taskCenterImpl.SaveTask(task); err != nil {
		return err
	}
	c.triggerTaskConsumer(task.TaskType)
	return nil
}

func (c *clientStruct) TriggerTaskConsumer(taskType string) {
	_ = c.nc.Publish(EvtPrefix+"consumer", []byte(taskType))
}

func (c *clientStruct) subscribeTaskConsumer() {
	_, _ = c.nc.Subscribe(EvtPrefix+"consumer", func(msg *nats.Msg) {
		taskType := string(msg.Data)
		if taskType == "" {
			return
		}
		if _, ok := c.taskTypeMap.Load(taskType); !ok {
			return
		}
		c.triggerTaskConsumer(taskType)
	})
}

func (c *clientStruct) triggerTaskConsumer(taskType string) {
	safeGo(func() {
		if _, ok := c.taskConsumers.Load(taskType); ok {
			return
		}
		c.taskConsumers.Store(taskType, struct{}{})
		defer c.taskConsumers.Delete(taskType)

		v, ok := c.taskTypeMap.Load(taskType)
		if !ok {
			return
		}
		taskTypeDO := v.(*TaskTypeDO)
		limit := taskTypeDO.Limit
		if limit <= 0 {
			limit = 10
		}
		tokenCh := make(chan struct{}, limit)
		for i := 0; i < limit; i++ {
			tokenCh <- struct{}{}
		}
		for {
			<-tokenCh
			tasks := c.taskCenterImpl.ExtractTasksByType(taskTypeDO.TaskType, TaskStatusWaiting, 1)
			if len(tasks) == 0 {
				return
			}
			task := tasks[0]

			var processor Processor
			if taskTypeDO.NewInstance {
				th, ok := c.taskHandlers.Load(taskTypeDO.TaskType)
				if !ok {
					return
				}
				configProcessorType := reflect.TypeOf(th)
				if configProcessorType.Kind() == reflect.Ptr {
					configProcessorType = configProcessorType.Elem()
				}
				processor = reflect.New(configProcessorType).Interface().(Processor)
				if task.Payload != nil {
					_ = json.Unmarshal(*task.Payload, processor)
				}
			} else {
				v, ok = c.taskHandlers.Load(taskTypeDO.TaskType)
				if !ok {
					return
				}
				processor = v.(Processor)
			}

			task.handler = c.taskCenterImpl
			task.taskTypeDO = taskTypeDO
			c.consumeTask(&task, processor, tokenCh)
		}
	})
}

func (c *clientStruct) consumeTask(task *TaskDO, processor Processor, tokenCh chan struct{}) {
	var isResume bool
	if task.CurrentContent == nil {
		task.StartAt = time.Now().UnixMilli()
	} else {
		isResume = true
	}

	task.Status = TaskStatusProcessing
	_ = task.updateTask()

	doneChan := make(chan struct{})
	task.currentCtx, task.currentCancel = context.WithCancel(context.Background())
	task.sub, _ = c.nc.Subscribe(task.EventUUID(), func(msg *nats.Msg) {
		switch TaskStatus(msg.Data) {
		case TaskStatusPaused:
			pauseProcessor, ok := processor.(PauseProcessor)
			if !ok {
				return
			}
			if task.Progress != nil && task.Progress.notifyTicker != nil {
				task.Progress.notifyTicker.Stop()
			}
			if task.currentCancel != nil {
				task.currentCancel()
			}
			<-doneChan
			err := pauseProcessor.Pause(task)
			if err == nil {
				task.Status = TaskStatusPaused
				_ = task.updateTask()
			}
		case TaskStatusCancel:
			cancelProcessor, ok := processor.(CancelProcessor)
			if !ok {
				return
			}
			if task.Progress != nil && task.Progress.notifyTicker != nil {
				task.Progress.notifyTicker.Stop()
			}
			if task.currentCancel != nil {
				task.currentCancel()
			}
			err := cancelProcessor.Cancel(task)
			if err == nil {
				task.EndAt = time.Now().UnixMilli()
				task.Status = TaskStatusCancel
				task.CurrentContent = nil
				_ = task.updateTask()
			}
		}
	})

	safeGo(func() {
		defer func() {
			tokenCh <- struct{}{}
		}()

		var err error
		if isResume {
			resumeProcessor, ok := processor.(ResumeProcessor)
			if ok {
				err = resumeProcessor.Resume(task)
			} else {
				runProcessor, _ := processor.(RunProcessor)
				err = runProcessor.Run(task)
			}
		} else {
			runProcessor, _ := processor.(RunProcessor)
			err = runProcessor.Run(task)
		}

		close(doneChan)
		if task.sub != nil {
			_ = task.sub.Unsubscribe()
			task.sub = nil
		}
		select {
		case <-task.currentCtx.Done():
			return
		default:
			if task.Progress != nil && task.Progress.notifyTicker != nil {
				task.Progress.notifyTicker.Stop()
			}
			if err != nil {
				task.Status = TaskStatusFailed
				task.ErrMsg = err.Error()
			} else {
				task.Status = TaskStatusSuccess
			}
			task.EndAt = time.Now().UnixMilli()
			_ = task.updateTask()
		}
	})
}

type NotifyType int

const (
	ToUser NotifyType = iota + 1
	ToUGN
	ToAll
)

func (c *clientStruct) publish(now int64, p *Progress) {
	if p == nil || p.Total == 0 {
		return
	}

	p.flush(now)
	c.taskCenterImpl.PublishProgress(p.ugn, p.taskTypeDO.NotifyType, p)
}
