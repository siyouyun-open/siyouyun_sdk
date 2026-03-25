package taskcenter

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

type TaskStatus string

const (
	TaskStatusWaiting    TaskStatus = "waiting"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusPaused     TaskStatus = "paused"
	TaskStatusSuccess    TaskStatus = "success"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancel     TaskStatus = "cancel"
)

const EvtPrefix = "siyou_task_center."

type AbilityFlag int

const (
	HandlerProcessing AbilityFlag = 0x01 << iota
	HandlerPause
	HandlerResume
	HandlerCancel
	HandlerForceRemove
)

type TaskTypeDO struct {
	TaskType string `json:"taskType"`
	TaskName string `json:"taskName"`

	NotifyType NotifyType `json:"notifyType"`

	NotifyTimeInterval    int64   `json:"notifyTimeInterval"`
	NotifyPercentInterval float64 `json:"notifyPercentInterval"`

	NotifyIncrement          bool   `json:"notifyIncrement"`
	NotifyIncrementTotalTmpl string `json:"notifyIncrementTotalTmpl"`

	NotifyRate bool     `json:"notifyRate"`
	RateType   RateType `json:"rateType"`

	Limit int `json:"limit"`

	AbilityFlag int `json:"abilityFlag"`

	Persistent bool `json:"persistent"`

	NewInstance bool `json:"-"`
}

func (t *TaskTypeDO) valid() error {
	if t.TaskType == "" || t.TaskName == "" {
		return errors.New("taskType is empty")
	}
	if t.NotifyType == 0 {
		t.NotifyType = ToUGN
	}
	if t.RateType == 0 {
		t.RateType = RateTypeUnit
	}
	return nil
}

type TaskDO struct {
	Id             uint64                    `json:"id"`
	UGN            *utils.UserGroupNamespace `json:"ugn"`
	UUID           string                    `json:"uuid"`
	TaskType       string                    `json:"taskType"`
	TaskTitle      string                    `json:"taskTitle"`
	CurrentContent *json.RawMessage          `json:"currentContent"`
	Payload        *json.RawMessage          `json:"payload"`
	Status         TaskStatus                `json:"status"`
	Deleted        bool                      `json:"deleted"`
	StartAt        int64                     `json:"startAt"`
	EndAt          int64                     `json:"endAt"`
	Progress       *Progress                 `json:"progress"`
	ErrMsg         string                    `json:"errMsg"`
	I18n           string                    `json:"i18n"`

	handler       TaskCenterInterface
	taskTypeDO    *TaskTypeDO
	sub           *nats.Subscription
	currentCtx    context.Context
	currentCancel context.CancelFunc
	tempVar       any
}

func (t *TaskDO) valid() error {
	if t.UGN == nil {
		return errors.New("ugn is empty")
	}
	if t.TaskType == "" {
		return errors.New("taskType is empty")
	}
	return nil
}

func (t *TaskDO) updateTask() error {
	if t.Progress == nil {
		t.newProgress(0, 0, false)
	}
	t.Progress.Status = t.Status
	if t.Progress.ugn == nil {
		t.Progress.ugn = t.UGN
		t.Progress.taskTypeDO = t.taskTypeDO
	}
	Client.publish(time.Now().UnixMilli(), t.Progress)
	if t.taskTypeDO != nil {
		_ = t.handler.PublishTaskStatusChange(t, t.taskTypeDO.NotifyType)
		return t.handler.UpdateTask(t, t.taskTypeDO.Persistent)
	}
	_ = t.handler.PublishTaskStatusChange(t, ToUGN)
	return t.handler.UpdateTask(t, false)
}

func (t *TaskDO) EventUUID() string {
	return EvtPrefix + t.UUID
}

func (t *TaskDO) GetTaskCtx() context.Context {
	return t.currentCtx
}

func (t *TaskDO) SaveTempVariable(v any) {
	t.tempVar = v
}

func (t *TaskDO) RemoveTempVariable() {
	t.tempVar = nil
}

func (t *TaskDO) GetTempVariable() any {
	return t.tempVar
}

func (t *TaskDO) SaveCurrentContent(v any) {
	rawContent, _ := json.Marshal(v)
	t.CurrentContent = (*json.RawMessage)(&rawContent)
}

func (t *TaskDO) GetTaskType() *TaskTypeDO {
	return t.taskTypeDO
}

type TaskDOBuilder struct {
	task *TaskDO
}

func NewTaskDOBuilder() *TaskDOBuilder {
	return &TaskDOBuilder{task: &TaskDO{}}
}

func (b *TaskDOBuilder) UGN(ugn *utils.UserGroupNamespace) *TaskDOBuilder {
	b.task.UGN = ugn
	return b
}

func (b *TaskDOBuilder) TaskType(taskType string) *TaskDOBuilder {
	b.task.TaskType = taskType
	return b
}

func (b *TaskDOBuilder) TaskTitle(taskTitle string) *TaskDOBuilder {
	b.task.TaskTitle = taskTitle
	return b
}

func (b *TaskDOBuilder) TaskUUID(uuid string) *TaskDOBuilder {
	b.task.UUID = uuid
	return b
}

func (b *TaskDOBuilder) Payload(payload any) *TaskDOBuilder {
	marshal, _ := json.Marshal(payload)
	b.task.Payload = (*json.RawMessage)(&marshal)
	return b
}

func (b *TaskDOBuilder) I18n(i18n string) *TaskDOBuilder {
	b.task.I18n = i18n
	return b
}

func (b *TaskDOBuilder) Build() *TaskDO {
	return b.task
}

type TaskTypeDOBuilder struct {
	taskTypeDO *TaskTypeDO
}

func NewTaskTypeDOBuilder() *TaskTypeDOBuilder {
	return &TaskTypeDOBuilder{taskTypeDO: &TaskTypeDO{}}
}

func (b *TaskTypeDOBuilder) TaskType(taskType string) *TaskTypeDOBuilder {
	b.taskTypeDO.TaskType = taskType
	return b
}

func (b *TaskTypeDOBuilder) TaskName(taskName string) *TaskTypeDOBuilder {
	b.taskTypeDO.TaskName = taskName
	return b
}

func (b *TaskTypeDOBuilder) NotifyType(notifyType NotifyType) *TaskTypeDOBuilder {
	b.taskTypeDO.NotifyType = notifyType
	return b
}

func (b *TaskTypeDOBuilder) NotifyTimeInterval(notifyTimeInterval int64) *TaskTypeDOBuilder {
	b.taskTypeDO.NotifyTimeInterval = notifyTimeInterval
	return b
}

func (b *TaskTypeDOBuilder) NotifyPercentInterval(notifyPercentInterval float64) *TaskTypeDOBuilder {
	b.taskTypeDO.NotifyPercentInterval = notifyPercentInterval
	return b
}

func (b *TaskTypeDOBuilder) NotifyIncrement(notifyIncrement bool, notifyIncrementTotalTmpl string) *TaskTypeDOBuilder {
	b.taskTypeDO.NotifyIncrement = notifyIncrement
	b.taskTypeDO.NotifyIncrementTotalTmpl = notifyIncrementTotalTmpl
	return b
}

func (b *TaskTypeDOBuilder) NotifyRate(notifyRate bool) *TaskTypeDOBuilder {
	b.taskTypeDO.NotifyRate = notifyRate
	return b
}

func (b *TaskTypeDOBuilder) RateType(rateType RateType) *TaskTypeDOBuilder {
	b.taskTypeDO.RateType = rateType
	return b
}

func (b *TaskTypeDOBuilder) Limit(limit int) *TaskTypeDOBuilder {
	b.taskTypeDO.Limit = limit
	return b
}

func (b *TaskTypeDOBuilder) Persistent(persistent bool) *TaskTypeDOBuilder {
	b.taskTypeDO.Persistent = persistent
	return b
}

func (b *TaskTypeDOBuilder) NewInstance(newInstance bool) *TaskTypeDOBuilder {
	b.taskTypeDO.NewInstance = newInstance
	return b
}

func (b *TaskTypeDOBuilder) Build() *TaskTypeDO {
	return b.taskTypeDO
}
