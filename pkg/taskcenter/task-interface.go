package taskcenter

import "github.com/siyouyun-open/siyouyun_sdk/pkg/utils"

type TaskCenterInterface interface {
	SaveTaskType(taskType *TaskTypeDO) error
	SaveTask(task *TaskDO) error
	ExtractTasksByType(taskType string, status TaskStatus, limit int) []TaskDO
	UpdateTask(task *TaskDO, persistent bool) error
	PublishTaskStatusChange(task *TaskDO, notifyType NotifyType) error
	PublishProgress(ugn *utils.UserGroupNamespace, notifyType NotifyType, p *Progress)
	HandleInterruptTask()
}

type Processor interface {
	Config() *TaskTypeDO
}

type RunProcessor interface {
	Run(taskDO *TaskDO) error
}

type ResumeProcessor interface {
	Resume(taskDO *TaskDO) error
}

type PauseProcessor interface {
	Pause(taskDO *TaskDO) error
}

type CancelProcessor interface {
	Cancel(taskDO *TaskDO) error
}

type ForceRemoveProcessor interface {
	ForceRemove(taskDO *TaskDO) error
}
