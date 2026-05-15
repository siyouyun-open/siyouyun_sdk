package taskcenter

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

// TaskCenterGateway is the gateway implementation of the sub-task center,
// which delegates persistence and notifications to the OS via MessageBroker.
// The sub-task center does not directly access the database or WebSocket;
// all operations are forwarded to the OS through the message broker.
type TaskCenterGateway struct {
	broker MessageBroker
	base   *baseClient
}

// progressRequest is the progress push request.
// The Gateway sends it to the OS via the broker,
// and the OS calls TaskCenterInterface.PublishProgress to push it to the frontend.
type progressRequest struct {
	UGN         *utils.UserGroupNamespace `json:"ugn"`
	NotifyScope NotifyScope               `json:"notifyScope"`
	Progress    *Progress                 `json:"progress"`
}

// statusChangeRequest is the status change notification request.
// The Gateway sends it to the OS via the broker,
// and the OS calls TaskCenterInterface.PublishTaskStatusChange to push an in-app notification.
type statusChangeRequest struct {
	Task        *TaskDO     `json:"task"`
	NotifyScope NotifyScope `json:"notifyScope"`
}

// PublishProgress pushes progress to the OS via the broker, and the OS pushes it to the frontend.
func (g *TaskCenterGateway) PublishProgress(ugn *utils.UserGroupNamespace, notifyScope NotifyScope, p *Progress) {
	req := &progressRequest{
		UGN:         ugn,
		NotifyScope: notifyScope,
		Progress:    p,
	}
	data, _ := json.Marshal(req)
	_ = g.broker.Publish(TopicOSProgress, data)
}

// PublishTaskStatusChange pushes the status change notification to the OS via the broker,
// and the OS pushes an in-app notification.
func (g *TaskCenterGateway) PublishTaskStatusChange(task *TaskDO, notifyScope NotifyScope) error {
	req := &statusChangeRequest{
		Task:        task,
		NotifyScope: notifyScope,
	}
	data, _ := json.Marshal(req)
	_ = g.broker.Publish(TopicOSStatusChange, data)
	return nil
}

// SaveTask remotely requests the OS to save the task via the broker.
func (g *TaskCenterGateway) SaveTask(task *TaskDO) error {
	data, _ := json.Marshal(task)
	msg, err := g.broker.Request(TopicOSSaveTask, data, 30*time.Second)
	if err != nil {
		return err
	}
	resp, err := parseBrokerResponse(msg.Data)
	if err != nil {
		return err
	}
	if !isBrokerSuccess(resp) {
		return fmt.Errorf("%s", resp.Msg)
	}
	return nil
}

// UpdateTask remotely requests the OS to update the task via the broker.
func (g *TaskCenterGateway) UpdateTask(task *TaskDO, persistent bool) error {
	data, _ := json.Marshal(task)
	msg, err := g.broker.Request(TopicOSUpdateTask, data, 30*time.Second)
	if err != nil {
		return err
	}
	resp, err := parseBrokerResponse(msg.Data)
	if err != nil {
		return err
	}
	if !isBrokerSuccess(resp) {
		return fmt.Errorf("%s", resp.Msg)
	}
	return nil
}

// GetTask remotely requests the OS to retrieve a task via the broker.
func (g *TaskCenterGateway) GetTask(uuid string) (*TaskDO, error) {
	msg, err := g.broker.Request(TopicOSGetTask, []byte(uuid), 30*time.Second)
	if err != nil {
		return nil, err
	}
	resp, err := parseBrokerResponse(msg.Data)
	if err != nil {
		return nil, err
	}
	if !isBrokerSuccess(resp) {
		return nil, fmt.Errorf("%s", resp.Msg)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("task not found")
	}
	taskBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, err
	}
	var task TaskDO
	if err := json.Unmarshal(taskBytes, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// DeleteTask remotely requests the OS to delete a task via the broker.
func (g *TaskCenterGateway) DeleteTask(uuid string, soft bool) error {
	req := map[string]any{
		"uuid": uuid,
		"soft": soft,
	}
	data, _ := json.Marshal(req)
	msg, err := g.broker.Request(TopicOSDeleteTask, data, 30*time.Second)
	if err != nil {
		return err
	}
	resp, err := parseBrokerResponse(msg.Data)
	if err != nil {
		return err
	}
	if !isBrokerSuccess(resp) {
		return fmt.Errorf("%s", resp.Msg)
	}
	return nil
}

// SaveTaskType remotely requests the OS to persist the task type definition via the broker.
func (g *TaskCenterGateway) SaveTaskType(taskType *TaskTypeDO) error {
	data, _ := json.Marshal(taskType)
	msg, err := g.broker.Request(TopicOSSaveTaskType, data, 30*time.Second)
	if err != nil {
		return err
	}
	resp, err := parseBrokerResponse(msg.Data)
	if err != nil {
		return err
	}
	if !isBrokerSuccess(resp) {
		return fmt.Errorf("%s", resp.Msg)
	}
	return nil
}

func (g *TaskCenterGateway) ExtractTasksByType(taskType string, status TaskStatus, limit int) []TaskDO {
	req := map[string]any{
		"taskType": taskType,
		"status":   string(status),
		"limit":    limit,
	}
	data, _ := json.Marshal(req)
	msg, err := g.broker.Request(TopicOSExtractTasks, data, 30*time.Second)
	if err != nil {
		return nil
	}
	resp, err := parseBrokerResponse(msg.Data)
	if err != nil {
		return nil
	}
	if !isBrokerSuccess(resp) {
		return nil
	}
	if resp.Data == nil {
		return nil
	}
	taskBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return nil
	}
	var tasks []TaskDO
	if err := json.Unmarshal(taskBytes, &tasks); err != nil {
		return nil
	}
	return tasks
}

// InitGateway initializes the Gateway (sub-task center) mode.
// The sub-task center does not directly access the database or WebSocket;
// all operations are delegated to the OS through the message broker.
// It automatically subscribes to: task consumer events and operation requests.
func InitGateway(owner string, broker MessageBroker) {
	gatewayOnce.Do(func() {
		gateway := &TaskCenterGateway{broker: broker}
		Client = &baseClient{
			broker:        broker,
			taskConsumers: &sync.Map{},
			taskTypeMap:   &sync.Map{},
			taskHandlers:  &sync.Map{},
			owner:         owner,
			publisher:     gateway,
		}
		gateway.base = Client
		Client.subscribeTaskConsumer()
		Client.subscribeOperations()
	})
}

// subscribeOperations subscribes to task operation requests (cancel/pause/resume/delete/force delete).
// Only the Gateway needs to call this method: the OS is the unified entry point for all task operations.
// When an OS operation targets a task belonging to the Gateway,
// it sends a Request via the broker to the "{owner}.operation.{op}" subject.
// The Gateway receives it, looks up the local Processor, executes the operation, and responds.
func (c *baseClient) subscribeOperations() {
	operations := []TaskOp{OpCancel, OpPause, OpResume, OpRemove, OpForceRemove}
	for _, op := range operations {
		subject := fmt.Sprintf(TopicOperationFormat, c.owner, op)
		_, _ = c.broker.Subscribe(subject, func(msg *Msg) {
			var req TaskOperationRequest
			if err := json.Unmarshal(msg.Data, &req); err != nil {
				brokerRespondError(msg, "invalid request")
				return
			}

			processor, err := c.GetTaskProcessor(req.TaskType)
			if err != nil {
				brokerRespondError(msg, err.Error())
				return
			}

			taskTypeDO, tErr := c.GetTaskType(req.TaskType)
			if tErr != nil {
				brokerRespondError(msg, tErr.Error())
				return
			}
			if !taskTypeDO.CanExecute(req.Operation) {
				brokerRespondError(msg, "operation not supported")
				return
			}

			task, err := c.publisher.GetTask(req.TaskUUID)
			if err != nil || task == nil {
				brokerRespondError(msg, "task not found")
				return
			}

			if execErr := c.applyOperation(req.Operation, task, processor, taskTypeDO); execErr != nil {
				brokerRespondError(msg, execErr.Error())
				return
			}

			brokerRespondSuccess(msg, nil)
		})
	}
}
