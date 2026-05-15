package taskcenter

import "github.com/siyouyun-open/siyouyun_sdk/pkg/utils"

const (
	OwnerOS = "os"
)

// TaskOp represents a task operation type.
type TaskOp string

const (
	OpRun         TaskOp = "run"
	OpCancel      TaskOp = "cancel"
	OpPause       TaskOp = "pause"
	OpResume      TaskOp = "resume"
	OpRetry       TaskOp = "retry"
	OpRemove      TaskOp = "remove"
	OpForceRemove TaskOp = "force_remove"
)

// TaskOperationRequest is a task operation request used for routing operation commands across task centers.
type TaskOperationRequest struct {
	RequestID string `json:"requestID"`
	TaskUUID  string `json:"taskUUID"`
	TaskType  string `json:"taskType"`
	// Owner is the identifier of the task center that owns the task, used for operation routing on the OS side.
	// It is populated from TaskDO.Owner by CancelTask/PauseTask methods,
	// and ExecuteOperation uses it to determine whether to execute locally or forward remotely,
	// without requiring an additional database query.
	Owner     string `json:"owner"`
	Operation TaskOp `json:"operation"`
	Source    string `json:"source"`
	Timestamp int64  `json:"timestamp"`
}

// TaskCenterInterface is the persistence and notification layer interface for the task center.
// This interface is only implemented by the OS (main task center),
// which has access to the database and WebSocket push capabilities.
// Sub-task centers (Gateway mode) do not implement this interface;
// instead, they delegate to the OS via MessageBroker (e.g., NATS).
type TaskCenterInterface interface {
	// SaveTaskType persists the task type definition to the database.
	SaveTaskType(taskType *TaskTypeDO) error
	// GetTaskType retrieves the task type definition by its identifier.
	GetTaskType(taskType string) (*TaskTypeDO, error)
	// SaveTask persists a new task to the database.
	SaveTask(task *TaskDO) error
	// GetTask retrieves a task by its UUID.
	GetTask(uuid string) (*TaskDO, error)
	// ExtractTasksByType queries tasks by type and status for the consumer loop.
	ExtractTasksByType(taskType string, status TaskStatus, limit int) []TaskDO
	// UpdateTask updates a task in the database.
	// The persistent parameter indicates whether the task data needs to be persisted.
	UpdateTask(task *TaskDO, persistent bool) error
	// DeleteTask deletes or soft-deletes a task from the database by UUID.
	// If soft is true, it performs a soft delete (sets the deleted flag);
	// if false, it performs a hard delete.
	DeleteTask(uuid string, soft bool) error
	// PublishProgress pushes task progress to the frontend via WebSocket/SSE.
	PublishProgress(ugn *utils.UserGroupNamespace, notifyScope NotifyScope, p *Progress)
	// PublishTaskStatusChange notifies the frontend of task status changes.
	PublishTaskStatusChange(task *TaskDO, notifyScope NotifyScope) error
	// HandleInterruptTask handles interrupted tasks
	// (e.g., tasks still in processing state after a process restart).
	HandleInterruptTask()
}

// Processor is the core interface for task processors. All task types must implement it.
// Config() returns the task type definition, which the framework uses to register
// the task type and compute the ability flags.
type Processor interface {
	Config() *TaskTypeDO
}

// RunProcessor is the execution processor. Task types implementing this interface support async execution.
type RunProcessor interface {
	Run(taskDO *TaskDO) error
}

// ResumeProcessor is the resume processor. Task types implementing this interface support resuming from a paused state.
type ResumeProcessor interface {
	Resume(taskDO *TaskDO) error
}

// PauseProcessor is the pause processor. Task types implementing this interface support pausing during execution.
type PauseProcessor interface {
	Pause(taskDO *TaskDO) error
}

// CancelProcessor is the cancel processor. Task types implementing this interface support canceling non-running tasks.
type CancelProcessor interface {
	Cancel(taskDO *TaskDO) error
}

// ForceRemoveProcessor is the force remove processor. Task types implementing this interface support cleaning up external resources during deletion.
type ForceRemoveProcessor interface {
	ForceRemove(taskDO *TaskDO) error
}
