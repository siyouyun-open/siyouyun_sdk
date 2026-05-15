package taskcenter

import (
	"fmt"
	"time"
)

// applyOperation executes the specific task operation logic based on the operation type.
// This method is called by the task center that owns the task's Processor
// (either a local OS task or a Gateway after receiving a remote operation request),
// so it can directly use triggerTaskConsumer (local trigger) without broadcasting.
//
// For running tasks (processing): it publishes an event via the broker to notify the consumer,
// which is responsible for:
//   1. Canceling the context / stopping the ticker
//   2. Calling the Processor's Cancel/Pause hook for cleanup
//   3. Updating the task status and persisting it
//
// For non-running tasks: the framework manages the status change directly.
// The Processor's Cancel/Pause hooks are called only as cleanup hooks
// (most implementations are no-ops), and status updates are handled uniformly by the framework.
// Each status change sends a progress notification via PublishProgress
// and an in-app notification via PublishTaskStatusChange.
//
// OpResume does not directly call Processor.Resume; instead, it sets the task status to waiting
// and triggers the consumer. When the consumer detects CurrentContent != nil,
// it calls ResumeProcessor.Resume to restore state before executing.
// This makes Resume execution asynchronous and does not block the operation entry point.
func (c *baseClient) applyOperation(op TaskOp, task *TaskDO, processor Processor, taskTypeDO *TaskTypeDO) error {
	switch op {
	case OpCancel:
		if task.Status == TaskStatusProcessing {
			return c.broker.Publish(task.EventUUID(), []byte(TaskStatusCancel))
		}
		if p, ok := processor.(CancelProcessor); ok {
			opTask := *task
			opTask.handler = c.publisher
			opTask.taskTypeDO = taskTypeDO
			if err := p.Cancel(&opTask); err != nil {
				return err
			}
		}
		task.Status = TaskStatusCancel
		task.EndAt = time.Now().UnixMilli()
		task.CurrentContent = nil
		return c.updateTaskWithNotify(task, taskTypeDO)
	case OpPause:
		if task.Status == TaskStatusProcessing {
			return c.broker.Publish(task.EventUUID(), []byte(TaskStatusPaused))
		}
		if task.Status != TaskStatusWaiting {
			return fmt.Errorf("only waiting task can be paused, current status: %s", task.Status)
		}
		if p, ok := processor.(PauseProcessor); ok {
			opTask := *task
			opTask.handler = c.publisher
			opTask.taskTypeDO = taskTypeDO
			if err := p.Pause(&opTask); err != nil {
				return err
			}
		}
		task.Status = TaskStatusPaused
		return c.updateTaskWithNotify(task, taskTypeDO)
	case OpResume:
		if task.Status != TaskStatusPaused {
			return fmt.Errorf("only paused task can be resumed, current status: %s", task.Status)
		}
		task.Status = TaskStatusWaiting
		if err := c.updateTaskWithNotify(task, taskTypeDO); err != nil {
			return err
		}
		c.triggerTaskConsumer(task.TaskType)
		return nil
	case OpRemove:
		if task.Status == TaskStatusProcessing {
			_ = c.broker.Publish(task.EventUUID(), []byte(TaskStatusCancel))
		}
		c.notifyTaskStatusChange(task, taskTypeDO)
		if taskTypeDO.CanExecute(OpForceRemove) {
			return c.publisher.DeleteTask(task.UUID, true)
		}
		return c.publisher.DeleteTask(task.UUID, false)
	case OpForceRemove:
		if task.Status == TaskStatusProcessing {
			_ = c.broker.Publish(task.EventUUID(), []byte(TaskStatusCancel))
		}
		if p, ok := processor.(ForceRemoveProcessor); ok {
			opTask := *task
			opTask.handler = c.publisher
			opTask.taskTypeDO = taskTypeDO
			if err := p.ForceRemove(&opTask); err != nil {
				return err
			}
		}
		c.notifyTaskStatusChange(task, taskTypeDO)
		return c.publisher.DeleteTask(task.UUID, false)
	}
	return nil
}

// updateTaskWithNotify updates the task persistence and publishes progress and in-app status change
// notifications. It is used for all non-processing task status change scenarios in applyOperation,
// ensuring that the frontend receives progress notifications via WebSocket
// and perceives status changes via in-app notifications.
func (c *baseClient) updateTaskWithNotify(task *TaskDO, taskTypeDO *TaskTypeDO) error {
	c.notifyTaskStatusChange(task, taskTypeDO)
	return c.publisher.UpdateTask(task, true)
}

// notifyTaskStatusChange publishes progress and in-app status change notifications.
// Progress notification (PublishProgress) pushes Progress data for the frontend to update the progress bar UI.
// In-app notification (PublishTaskStatusChange) pushes status change events for the frontend
// to display messages (e.g., task success/failure/cancellation).
func (c *baseClient) notifyTaskStatusChange(task *TaskDO, taskTypeDO *TaskTypeDO) {
	notifyScope := getNotifyScope(taskTypeDO)
	p := &Progress{
		Id:        task.UUID,
		TaskType:  task.TaskType,
		TaskTitle: task.TaskTitle,
		Status:    task.Status,
		Total:     0,
		Current:   0,
		Percent:   "0",
		Start:     task.StartAt,
		Cost:      0,
		Seq:       0,
	}
	if task.EndAt > 0 && task.StartAt > 0 {
		p.Cost = task.EndAt - task.StartAt
	}
	c.publisher.PublishProgress(task.UGN, notifyScope, p)
	_ = c.publisher.PublishTaskStatusChange(task, notifyScope)
}

// getNotifyScope retrieves the notification scope from TaskTypeDO, defaulting to ScopeUGN.
func getNotifyScope(taskTypeDO *TaskTypeDO) NotifyScope {
	if taskTypeDO != nil {
		return taskTypeDO.NotifyScope
	}
	return ScopeUGN
}
