package taskcenter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

// TaskStatus represents the task status type.
type TaskStatus string

const (
	// TaskStatusWaiting indicates the task is waiting and has been created but not yet started.
	TaskStatusWaiting TaskStatus = "waiting"
	// TaskStatusProcessing indicates the task is being executed.
	TaskStatusProcessing TaskStatus = "processing"
	// TaskStatusPaused indicates the task has been paused by the user and can be resumed.
	TaskStatusPaused TaskStatus = "paused"
	// TaskStatusSuccess indicates the task has completed successfully without errors.
	TaskStatusSuccess TaskStatus = "success"
	// TaskStatusFailed indicates the task has failed due to an error during execution.
	TaskStatusFailed TaskStatus = "failed"
	// TaskStatusCancel indicates the task has been canceled by the user.
	TaskStatusCancel TaskStatus = "cancel"
)

// AbilityFlag represents the processor capability flags, indicating which operations a Processor supports.
type AbilityFlag int

const (
	// HandlerProcessing indicates the processing capability, present by default on all processors.
	HandlerProcessing AbilityFlag = 0x01 << iota
	// HandlerPause indicates the pause capability, present on processors implementing PauseProcessor.
	HandlerPause
	// HandlerResume indicates the resume capability, present on processors implementing ResumeProcessor.
	HandlerResume
	// HandlerCancel indicates the cancel capability, present on processors implementing CancelProcessor.
	HandlerCancel
	// HandlerForceRemove indicates the force remove capability, present on processors implementing ForceRemoveProcessor.
	HandlerForceRemove
)

// TaskTypeDO defines the task type metadata and notification configuration for a category of tasks.
type TaskTypeDO struct {
	TaskType string `json:"taskType"`
	TaskName string `json:"taskName"`
	Owner    string `json:"owner"`
	// --- Notification scope ---
	// NotifyScope defines the notification scope, determining the target audience for progress push.
	NotifyScope NotifyScope `json:"notifyScope"`
	// --- Notification trigger strategy ---
	// NotifyTimeInterval is the periodic notification interval (milliseconds).
	// Progress is pushed at this interval.
	NotifyTimeInterval int64 `json:"notifyTimeInterval"`
	// NotifyPercentInterval is the percentage change threshold.
	// A notification is triggered when the progress change exceeds this value.
	NotifyPercentInterval float64 `json:"notifyPercentInterval"`
	// NotifyIncrement indicates whether incremental notification is enabled,
	// pushing a notification on every progress change.
	NotifyIncrement bool `json:"notifyIncrement"`
	// NotifyIncrementTotalTmpl is the template for describing the total in incremental notifications,
	// e.g., "processing {total} files".
	NotifyIncrementTotalTmpl string `json:"notifyIncrementTotalTmpl"`
	// --- Notification content enhancement ---
	// NotifyRate indicates whether to display the processing rate in notifications.
	NotifyRate bool `json:"notifyRate"`
	// RateType is the rate calculation type (e.g., by count or by byte), used with NotifyRate.
	RateType RateType `json:"rateType"`
	// --- Concurrency and persistence ---
	// Limit is the maximum concurrency for tasks of the same type.
	Limit int `json:"limit"`
	// AbilityFlag represents the processor capability flag, automatically computed by the framework during registration.
	AbilityFlag int `json:"abilityFlag"`
	// Persistent indicates whether to persist task data.
	Persistent bool `json:"persistent"`
	// NewInstance indicates whether to create a new instance each time (for non-singleton task types).
	NewInstance bool `json:"-"`
}

// valid validates the required fields of TaskTypeDO and sets defaults for NotifyScope and RateType.
func (t *TaskTypeDO) valid() error {
	if t.TaskType == "" || t.TaskName == "" {
		return errors.New("taskType or taskName is empty")
	}
	if t.NotifyScope == 0 {
		t.NotifyScope = ScopeUGN
	}
	if t.RateType == 0 {
		t.RateType = RateTypeUnit
	}
	return nil
}

// OwnerService returns the service identifier of the task type.
func (t *TaskTypeDO) OwnerService() string {
	return t.Owner
}

// CanExecute checks whether the current task type supports the specified operation.
// OpRun, OpRetry, and OpRemove are always supported; other operations require the corresponding capability flag.
func (t *TaskTypeDO) CanExecute(op TaskOp) bool {
	switch op {
	case OpCancel:
		return t.AbilityFlag&int(HandlerCancel) != 0
	case OpPause:
		return t.AbilityFlag&int(HandlerPause) != 0
	case OpResume:
		return t.AbilityFlag&int(HandlerResume) != 0
	case OpForceRemove:
		return t.AbilityFlag&int(HandlerForceRemove) != 0
	case OpRun, OpRetry, OpRemove:
		return true
	}
	return false
}

// TaskDO represents a task instance, containing basic task information, status, progress, and runtime control fields.
type TaskDO struct {
	// Id is the database auto-increment primary key.
	Id uint64 `json:"id"`
	// UGN is the user/group/namespace information for permission validation and progress push.
	UGN *utils.UserGroupNamespace `json:"ugn"`
	// UUID is the unique task identifier, auto-generated by the framework or specified by the caller.
	UUID string `json:"uuid"`
	// TaskType is the task type identifier, corresponding to TaskTypeDO.TaskType.
	TaskType string `json:"taskType"`
	// TaskTitle is the task title displayed on the frontend.
	TaskTitle string `json:"taskTitle"`
	// CurrentContent is the current execution context content, used for passing state during pause/resume.
	CurrentContent *json.RawMessage `json:"currentContent"`
	// Payload is the task parameters provided by the caller when creating the task.
	Payload *json.RawMessage `json:"payload"`
	// Status is the current task status.
	Status TaskStatus `json:"status"`
	// Deleted is the soft-delete flag; true indicates soft-deleted.
	Deleted bool `json:"deleted"`
	// StartAt is the task start time (millisecond timestamp).
	StartAt int64 `json:"startAt"`
	// EndAt is the task end time (millisecond timestamp), set on success/failure/cancellation.
	EndAt int64 `json:"endAt"`
	// Progress is the task progress information.
	Progress *Progress `json:"progress"`
	// ErrMsg is the error message, recording the reason for task failure.
	ErrMsg string `json:"errMsg"`
	// Owner is the identifier of the task center that owns the task, used for operation routing.
	// For tasks created by the OS, Owner is "os".
	// For tasks created by the Gateway, Owner is the Gateway's identifier.
	// The Owner directly indicates which task center owns the task without querying TaskTypeDO.
	Owner string `json:"owner"`
	// I18n is the internationalization identifier for multilingual frontend display.
	I18n string `json:"i18n"`

	// mu is the runtime state mutex, protecting concurrent access to runtime fields.
	mu sync.Mutex `json:"-"`
	// handler is the progress publisher, injected by the framework when consuming the task.
	handler ProgressPublisher `json:"-"`
	// taskTypeDO is the task type definition reference, injected by the framework when consuming the task.
	taskTypeDO *TaskTypeDO `json:"-"`
	// sub is the message subscription handle for receiving cancel/pause operation commands.
	sub Subscription `json:"-"`
	// currentCtx is the current execution context, used for canceling tasks via context.
	currentCtx context.Context `json:"-"`
	// currentCancel is the cancel function for the current execution context.
	currentCancel context.CancelFunc `json:"-"`
	// controlCh is the control command channel, reserved for future extensions.
	controlCh chan taskControl `json:"-"`
	// tempVar is a temporary variable for the Processor to store intermediate state during execution.
	tempVar any `json:"-"`
}

// valid validates the required fields of TaskDO.
func (t *TaskDO) valid() error {
	if t.UGN == nil {
		return errors.New("ugn is empty")
	}
	if t.TaskType == "" {
		return errors.New("taskType is empty")
	}
	return nil
}

// updateTask updates the task status and publishes progress and status change notifications.
// It first refreshes the progress information, then publishes the progress, status change,
// and persists the task via ProgressPublisher.
func (t *TaskDO) updateTask() error {
	t.mu.Lock()
	if t.Progress == nil {
		t.newProgress(0, 0, false)
	}
	t.Progress.Status = t.Status
	if t.Progress.ugn == nil {
		t.Progress.ugn = t.UGN
		t.Progress.taskTypeDO = t.taskTypeDO
	}
	now := time.Now().UnixMilli()
	t.Progress.flush(now)
	progressCopy := *t.Progress
	snapshot := TaskDO{
		Id:             t.Id,
		UGN:            t.UGN,
		UUID:           t.UUID,
		TaskType:       t.TaskType,
		TaskTitle:      t.TaskTitle,
		CurrentContent: t.CurrentContent,
		Payload:        t.Payload,
		Status:         t.Status,
		Deleted:        t.Deleted,
		StartAt:        t.StartAt,
		EndAt:          t.EndAt,
		Progress:       &progressCopy,
		ErrMsg:         t.ErrMsg,
		Owner:          t.Owner,
		I18n:           t.I18n,
	}
	persistent := false
	if t.taskTypeDO != nil {
		persistent = t.taskTypeDO.Persistent
	}
	ugn := t.UGN
	notifyScope := ScopeUGN
	if t.taskTypeDO != nil {
		notifyScope = t.taskTypeDO.NotifyScope
	}
	t.mu.Unlock()

	t.handler.PublishProgress(ugn, notifyScope, &progressCopy)
	_ = t.handler.PublishTaskStatusChange(&snapshot, notifyScope)
	return t.handler.UpdateTask(&snapshot, persistent)
}

// EventUUID returns the task event topic identifier in the format "siyou_task.event.{uuid}",
// used for subscribing to runtime operation commands such as cancel/pause.
func (t *TaskDO) EventUUID() string {
	return fmt.Sprintf(TopicTaskEventFormat, t.UUID)
}

// GetTaskCtx returns the current execution context of the task; the Processor can use it to listen for cancellation signals.
func (t *TaskDO) GetTaskCtx() context.Context {
	return t.currentCtx
}

// SaveTempVariable saves a temporary variable for the Processor to store intermediate state during execution.
func (t *TaskDO) SaveTempVariable(v any) {
	t.tempVar = v
}

// RemoveTempVariable clears the temporary variable.
func (t *TaskDO) RemoveTempVariable() {
	t.tempVar = nil
}

// GetTempVariable retrieves the temporary variable.
func (t *TaskDO) GetTempVariable() any {
	return t.tempVar
}

// SaveCurrentContent saves the current execution context content for passing state during pause/resume.
func (t *TaskDO) SaveCurrentContent(v any) {
	rawContent, _ := json.Marshal(v)
	t.CurrentContent = (*json.RawMessage)(&rawContent)
}

// GetTaskType returns the task type definition.
func (t *TaskDO) GetTaskType() *TaskTypeDO {
	return t.taskTypeDO
}

// TaskDOBuilder is the builder for TaskDO, providing a fluent API for creating task instances.
type TaskDOBuilder struct {
	task *TaskDO
}

// NewTaskDOBuilder creates a new TaskDOBuilder instance.
func NewTaskDOBuilder() *TaskDOBuilder {
	return &TaskDOBuilder{task: &TaskDO{}}
}

// UGN sets the UGN parameter on TaskDOBuilder.
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

// Build builds and returns the TaskDO instance.
func (b *TaskDOBuilder) Build() *TaskDO {
	return b.task
}

// TaskTypeDOBuilder is the builder for TaskTypeDO, providing a fluent API for creating task type definitions.
type TaskTypeDOBuilder struct {
	taskTypeDO *TaskTypeDO
}

// NewTaskTypeDOBuilder creates a new TaskTypeDOBuilder instance.
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

func (b *TaskTypeDOBuilder) NotifyScope(notifyScope NotifyScope) *TaskTypeDOBuilder {
	b.taskTypeDO.NotifyScope = notifyScope
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

func (b *TaskTypeDOBuilder) Owner(owner string) *TaskTypeDOBuilder {
	b.taskTypeDO.Owner = owner
	return b
}

// Build builds and returns the TaskTypeDO instance.
func (b *TaskTypeDOBuilder) Build() *TaskTypeDO {
	return b.taskTypeDO
}
