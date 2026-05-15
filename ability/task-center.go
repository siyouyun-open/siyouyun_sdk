package ability

import (
	"github.com/nats-io/nats.go"

	taskcenter "github.com/siyouyun-open/siyouyun_sdk/pkg/taskcenter"
)

type TaskCenter struct {
	nc *nats.Conn
}

func NewTaskCenter(appCode *string, nc *nats.Conn) *TaskCenter {
	tc := &TaskCenter{
		nc: nc,
	}
	taskcenter.InitGateway(*appCode, taskcenter.NewNATSBroker(nc))
	return tc
}

func (t *TaskCenter) Name() string {
	return "TaskCenter"
}

func (t *TaskCenter) IsReady() bool {
	return taskcenter.Client != nil
}

func (t *TaskCenter) Close() {
}

func (t *TaskCenter) RegisterTaskType(processor taskcenter.Processor) error {
	return taskcenter.Client.RegisterTaskType(processor)
}

func (t *TaskCenter) RequestTask(task *taskcenter.TaskDO) error {
	return taskcenter.Client.RequestTask(task)
}
