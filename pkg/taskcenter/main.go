package taskcenter

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	rj "github.com/siyouyun-open/siyouyun_sdk/pkg/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

const (
	// TopicPrefixTask is the task center topic prefix. All task-related topics use this prefix.
	TopicPrefixTask = "siyou_task."

	// TopicOSSaveTask Gateway → OS: save task (Request-Response)
	TopicOSSaveTask = TopicPrefixTask + "os.save_task"
	// TopicOSUpdateTask Gateway → OS: update task (Request-Response)
	TopicOSUpdateTask = TopicPrefixTask + "os.update_task"
	// TopicOSDeleteTask Gateway → OS: delete task (Request-Response)
	TopicOSDeleteTask = TopicPrefixTask + "os.delete_task"
	// TopicOSExtractTasks Gateway → OS: extract waiting tasks of a specified type (Request-Response)
	TopicOSExtractTasks = TopicPrefixTask + "os.extract_tasks"
	// TopicOSGetTask Gateway → OS: get a single task (Request-Response)
	TopicOSGetTask = TopicPrefixTask + "os.get_task"
	// TopicOSProgress Gateway → OS: push progress notification (Publish async)
	TopicOSProgress = TopicPrefixTask + "os.progress"
	// TopicOSStatusChange Gateway → OS: push task status change notification (Publish async)
	TopicOSStatusChange = TopicPrefixTask + "os.status_change"
	// TopicOSHeartbeat Gateway → OS: online probe, Gateway uses Request to check if OS is online
	TopicOSHeartbeat = TopicPrefixTask + "os.heartbeat"
	// TopicOSSaveTaskType Gateway → OS: persist task type definition (Request-Response)
	TopicOSSaveTaskType = TopicPrefixTask + "os.save_task_type"
	// TopicOperationFormat OS → Gateway: operation command topic format, "siyou_task.{owner}.operation.{op}"
	TopicOperationFormat = TopicPrefixTask + "%s.operation.%s"
	// TopicConsumerEvent is the task consumption trigger event,
	// broadcast to all nodes to check and consume tasks of a specified type.
	TopicConsumerEvent = TopicPrefixTask + "consumer"
	// TopicTaskEventFormat is the task runtime event topic format, "siyou_task.event.{taskUUID}".
	// Used by consumeTask to subscribe to cancel/pause operation commands.
	TopicTaskEventFormat = TopicPrefixTask + "event.%s"
)

// NotifyScope defines the notification scope, determining the target audience for progress push.
type NotifyScope int

const (
	// ScopeUser notifies user
	ScopeUser NotifyScope = iota + 1
	// ScopeUGN notifies namespace
	ScopeUGN
	// ScopeAll notifies all
	ScopeAll
)

// ProgressPublisher is the strategy interface for task persistence and progress publishing.
// OS mode is implemented by osTaskCenterImpl (directly delegates to TaskCenterInterface),
// Gateway mode is implemented by TaskCenterGateway (remotely requests the OS via broker).
// baseClient uses this interface to decouple the two roles, avoiding if-gateway conditionals.
type ProgressPublisher interface {
	PublishProgress(ugn *utils.UserGroupNamespace, notifyScope NotifyScope, p *Progress)
	PublishTaskStatusChange(task *TaskDO, notifyScope NotifyScope) error
	SaveTask(task *TaskDO) error
	UpdateTask(task *TaskDO, persistent bool) error
	GetTask(uuid string) (*TaskDO, error)
	DeleteTask(uuid string, soft bool) error
	SaveTaskType(taskType *TaskTypeDO) error
	ExtractTasksByType(taskType string, status TaskStatus, limit int) []TaskDO
}

// baseClient is the shared base client for both OS and Gateway modes,
// distinguishing the two roles through the ProgressPublisher strategy interface.
type baseClient struct {
	// broker is the abstract message broker interface for cross-node communication.
	broker MessageBroker
	// taskConsumers holds running task consumption goroutines.
	// sync.Map ensures only one consumer goroutine runs per task type at a time.
	taskConsumers *sync.Map
	// taskTypeMap is the task type registry, keyed by TaskType, valued by *TaskTypeDO (with AbilityFlag).
	taskTypeMap *sync.Map
	// taskHandlers is the task processor registry, keyed by TaskType, valued by Processor implementation.
	taskHandlers *sync.Map
	// owner is the identifier of the current task center.
	// "os" for the OS, and the service name for the Gateway.
	owner     string
	publisher ProgressPublisher
}

var (
	// Client is the global client instance, available after Init or InitGateway.
	Client      *baseClient
	initOnce    sync.Once
	gatewayOnce sync.Once
)

// IsOS returns whether the current client is in OS (main task center) mode.
func (c *baseClient) IsOS() bool {
	return c.owner == OwnerOS
}

// IsOSOnline checks whether the OS (main task center) is online.
// OS mode always returns true; Gateway mode sends a Request probe to the OS, and returns true if it responds.
func (c *baseClient) IsOSOnline() bool {
	if c.IsOS() {
		return true
	}
	_, err := c.broker.Request(TopicOSHeartbeat, nil, time.Second)
	return err == nil
}

// RegisterTaskType registers a task type processor.
// It validates the processor configuration, computes the ability flags,
// and persists the task type definition (in OS mode).
// After registration, it automatically triggers the consumption flow for that task type.
func (c *baseClient) RegisterTaskType(processor Processor) error {
	taskTypeDO := processor.Config()
	if taskTypeDO == nil {
		return errors.New("processor config is nil")
	}
	if err := taskTypeDO.valid(); err != nil {
		return err
	}
	taskTypeDO.Owner = c.owner

	if _, ok := processor.(RunProcessor); !ok {
		return errors.New("processor must implement RunProcessor")
	}
	if _, ok := processor.(CancelProcessor); !ok {
		return errors.New("processor must implement CancelProcessor")
	}

	var flag AbilityFlag
	flag |= HandlerProcessing
	flag |= HandlerCancel
	if _, ok := processor.(ResumeProcessor); ok {
		flag |= HandlerResume
	}
	if _, ok := processor.(PauseProcessor); ok {
		flag |= HandlerPause
	}
	if _, ok := processor.(ForceRemoveProcessor); ok {
		flag |= HandlerForceRemove
	}

	taskTypeDO.AbilityFlag = int(flag)
	c.taskTypeMap.Store(taskTypeDO.TaskType, taskTypeDO)
	c.taskHandlers.Store(taskTypeDO.TaskType, processor)

	saveCopy := *taskTypeDO
	utils.SafeGo(func() {
		_ = c.publisher.SaveTaskType(&saveCopy)
	})

	c.triggerTaskConsumer(taskTypeDO.TaskType)
	return nil
}

// GetTaskType retrieves the task type definition from the local cache.
func (c *baseClient) GetTaskType(taskType string) (*TaskTypeDO, error) {
	taskTypeDO, ok := c.taskTypeMap.Load(taskType)
	if !ok {
		return nil, errors.New("task type not exist")
	}
	return taskTypeDO.(*TaskTypeDO), nil
}

// GetTaskProcessor retrieves the task processor from the local cache.
func (c *baseClient) GetTaskProcessor(taskType string) (Processor, error) {
	processor, ok := c.taskHandlers.Load(taskType)
	if !ok {
		return nil, errors.New("task type not exist")
	}
	return processor.(Processor), nil
}

// RequestTask submits a new task. It validates the task fields, generates a UUID,
// persists the task, and triggers consumption.
func (c *baseClient) RequestTask(task *TaskDO) error {
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
	task.Owner = c.owner

	if err := c.publisher.SaveTask(task); err != nil {
		return err
	}
	c.triggerTaskConsumer(task.TaskType)
	return nil
}

// TriggerTaskConsumer broadcasts a task consumption trigger event via the broker,
// notifying all nodes to check and consume tasks of the specified type.
func (c *baseClient) TriggerTaskConsumer(taskType string) {
	_ = c.broker.Publish(TopicConsumerEvent, []byte(taskType))
}

// subscribeTaskConsumer subscribes to task consumer events.
// Both OS and Gateway need to call this method:
// When any node triggers consumption via TriggerTaskConsumer,
// all nodes that have registered the task type will be notified,
// and each node decides whether to start the consumption flow based on whether it has the Processor locally.
func (c *baseClient) subscribeTaskConsumer() {
	_, _ = c.broker.Subscribe(TopicConsumerEvent, func(msg *Msg) {
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

// triggerTaskConsumer starts the task consumption flow.
// It uses LoadOrStore to ensure only one consumer goroutine runs per task type at a time.
// It distributes tokens based on the concurrency limit (Limit),
// loops to extract waiting tasks, and hands them over to the Processor for execution.
func (c *baseClient) triggerTaskConsumer(taskType string) {
	utils.SafeGo(func() {
		if _, loaded := c.taskConsumers.LoadOrStore(taskType, struct{}{}); loaded {
			return
		}
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
			tasks := c.extractTasks(taskTypeDO.TaskType, TaskStatusWaiting, 1)
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
			task.handler = c.publisher
			task.taskTypeDO = taskTypeDO
			c.consumeTask(&task, processor, tokenCh)
		}
	})
}

// extractTasks extracts tasks of the specified type and status.
// In OS mode, it queries directly from the database.
// In Gateway mode, it remotely requests the OS via the broker.
func (c *baseClient) extractTasks(taskType string, status TaskStatus, limit int) []TaskDO {
	return c.publisher.ExtractTasksByType(taskType, status, limit)
}

type taskControl struct {
	operation TaskOp
}

// consumeTask consumes a single task: sets the status to processing, subscribes to operation commands,
// and starts the execution goroutine.
// After the execution goroutine completes, it updates the task status to success/failed.
// The operation command goroutine handles cancel/pause.
func (c *baseClient) consumeTask(task *TaskDO, processor Processor, tokenCh chan struct{}) {
	task.mu.Lock()
	var isResume bool
	if task.CurrentContent == nil {
		task.StartAt = time.Now().UnixMilli()
	} else {
		isResume = true
	}
	task.Status = TaskStatusProcessing
	task.currentCtx, task.currentCancel = context.WithCancel(context.Background())
	task.controlCh = make(chan taskControl, 1)
	task.mu.Unlock()

	_ = task.updateTask()

	sub, _ := c.broker.Subscribe(task.EventUUID(), func(msg *Msg) {
		switch TaskStatus(msg.Data) {
		case TaskStatusPaused:
			pauseProcessor, ok := processor.(PauseProcessor)
			if !ok {
				return
			}
			task.mu.Lock()
			if task.Progress != nil && task.Progress.notifyTicker != nil {
				task.Progress.notifyTicker.Stop()
			}
			if task.currentCancel != nil {
				task.currentCancel()
			}
			task.mu.Unlock()

			_ = pauseProcessor.Pause(task)

			task.mu.Lock()
			task.Status = TaskStatusPaused
			task.mu.Unlock()
			_ = task.updateTask()

		case TaskStatusCancel:
			cancelProcessor, ok := processor.(CancelProcessor)
			if !ok {
				return
			}
			task.mu.Lock()
			if task.Progress != nil && task.Progress.notifyTicker != nil {
				task.Progress.notifyTicker.Stop()
			}
			if task.currentCancel != nil {
				task.currentCancel()
			}
			task.mu.Unlock()

			_ = cancelProcessor.Cancel(task)

			task.mu.Lock()
			task.EndAt = time.Now().UnixMilli()
			task.Status = TaskStatusCancel
			task.CurrentContent = nil
			task.mu.Unlock()
			_ = task.updateTask()
		}
	})

	task.mu.Lock()
	task.sub = sub
	task.mu.Unlock()

	utils.SafeGo(func() {
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

		task.mu.Lock()
		var subToUnsub Subscription
		if task.sub != nil {
			subToUnsub = task.sub
			task.sub = nil
		}
		task.mu.Unlock()

		if subToUnsub != nil {
			_ = subToUnsub.Unsubscribe()
		}

		task.mu.Lock()
		ctxDone := false
		select {
		case <-task.currentCtx.Done():
			ctxDone = true
		default:
		}
		if task.currentCancel != nil {
			task.currentCancel()
		}
		if ctxDone {
			task.mu.Unlock()
			return
		}
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
		task.mu.Unlock()
		_ = task.updateTask()
	})
}

// publish refreshes the progress and publishes the notification, called periodically by the ticker.
func (c *baseClient) publish(now int64, p *Progress) {
	if p == nil {
		return
	}
	if p.Total == 0 && (p.taskTypeDO == nil || !p.taskTypeDO.NotifyIncrement) {
		return
	}
	p.flush(now)
	if p.taskTypeDO != nil {
		c.publisher.PublishProgress(p.ugn, p.taskTypeDO.NotifyScope, p)
	} else {
		c.publisher.PublishProgress(p.ugn, ScopeUGN, p)
	}
}

func brokerSuccessResponse(data any) []byte {
	resp := rj.SuccessResJsonWithData(&data)
	b, _ := json.Marshal(resp)
	return b
}

func brokerErrorResponse(errMsg string) []byte {
	resp := rj.ErrorResJsonWithMsg(errMsg)
	b, _ := json.Marshal(resp)
	return b
}

func brokerResponseFromError(err error) []byte {
	if err == nil {
		return brokerSuccessResponse(nil)
	}
	return brokerErrorResponse(err.Error())
}

func parseBrokerResponse(data []byte) (*rj.Response[any], error) {
	var resp rj.Response[any]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func isBrokerSuccess(resp *rj.Response[any]) bool {
	return resp.Code == sdkconst.Success
}

func brokerRespondError(msg *Msg, errMsg string) {
	_ = msg.Respond(brokerErrorResponse(errMsg))
}

func brokerRespondSuccess(msg *Msg, data any) {
	_ = msg.Respond(brokerSuccessResponse(data))
}
