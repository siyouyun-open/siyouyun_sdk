package ability

import (
	"errors"
	"strconv"
	"sync"

	"github.com/nats-io/nats.go"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	taskcenter "github.com/siyouyun-open/siyouyun_sdk/pkg/taskcenter"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

var taskCenterInitOnce sync.Once

type TaskCenter struct {
	nc          *nats.Conn
	taskGateway string
}

func NewTaskCenter(nc *nats.Conn) *TaskCenter {
	tc := &TaskCenter{
		nc:          nc,
		taskGateway: utils.GetOSServiceURL() + "/task",
	}
	if nc == nil {
		sdklog.Logger.Errorf("TaskCenter init err: nats conn is nil")
		return tc
	}
	taskCenterInitOnce.Do(func() {
		taskcenter.Init(nc, tc)
	})
	return tc
}

func (t *TaskCenter) Name() string {
	return "TaskCenter"
}

func (t *TaskCenter) IsReady() bool {
	if t == nil || t.nc == nil || !t.nc.IsConnected() {
		return false
	}
	return isOSServiceReady()
}

func (t *TaskCenter) Close() {
}

func (t *TaskCenter) RegisterTaskType(processor taskcenter.Processor) error {
	if taskcenter.Client == nil {
		return errors.New("task center not initialized")
	}
	taskcenter.Client.RegisterTaskType(processor)
	return nil
}

func (t *TaskCenter) RequestTask(task *taskcenter.TaskDO) error {
	if taskcenter.Client == nil {
		return errors.New("task center not initialized")
	}
	return taskcenter.Client.RequestTask(task)
}

func (t *TaskCenter) SaveTaskType(taskType *taskcenter.TaskTypeDO) error {
	api := t.taskGateway + "/type/save"
	resp := restclient.PostRequest[any](nil, api, nil, taskType)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (t *TaskCenter) SaveTask(task *taskcenter.TaskDO) error {
	api := t.taskGateway + "/save"
	resp := restclient.PostRequest[any](nil, api, nil, task)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (t *TaskCenter) ExtractTasksByType(taskType string, status taskcenter.TaskStatus, limit int) []taskcenter.TaskDO {
	api := t.taskGateway + "/extract"
	resp := restclient.GetRequest[[]taskcenter.TaskDO](nil, api, map[string]string{
		"taskType": taskType,
		"status":   string(status),
		"limit":    strconv.Itoa(limit),
	})
	if resp.Code != sdkconst.Success || resp.Data == nil {
		return nil
	}
	return *resp.Data
}

func (t *TaskCenter) UpdateTask(task *taskcenter.TaskDO, persistent bool) error {
	api := t.taskGateway + "/update"
	resp := restclient.PostRequest[any](nil, api, map[string]string{
		"persistent": strconv.FormatBool(persistent),
	}, task)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func (t *TaskCenter) PublishTaskStatusChange(task *taskcenter.TaskDO, notifyType taskcenter.NotifyType) error {
	api := t.taskGateway + "/status/change"
	resp := restclient.PostRequest[any](nil, api, map[string]string{
		"notifyType": strconv.Itoa(int(notifyType)),
	}, task)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

type commonNotify struct {
	EventName string `json:"eventName"`
	Payload   any    `json:"payload"`
	Persist   bool   `json:"persist,omitempty"`
}

func (t *TaskCenter) PublishProgress(ugn *utils.UserGroupNamespace, notifyType taskcenter.NotifyType, p *taskcenter.Progress) {
	if ugn == nil || p == nil {
		return
	}
	notify := &commonNotify{
		EventName: "server_progress_update",
		Payload:   p,
		Persist:   false,
	}

	switch notifyType {
	case taskcenter.ToAll:
		api := utils.GetOSServiceURL() + "/notify/publish/to/all"
		resp := restclient.PostRequest[any](nil, api, nil, notify)
		if resp.Code != sdkconst.Success {
			sdklog.Logger.Errorf("TaskCenter publish progress err: %v", resp.Msg)
		}
	case taskcenter.ToUser:
		api := utils.GetOSServiceURL() + "/notify/publish/to/user"
		resp := restclient.PostRequest[any](nil, api, map[string]string{
			"username": ugn.Username,
		}, notify)
		if resp.Code != sdkconst.Success {
			sdklog.Logger.Errorf("TaskCenter publish progress err: %v", resp.Msg)
		}
	default:
		api := utils.GetOSServiceURL() + "/notify/publish/to/user/namespace"
		resp := restclient.PostRequest[any](nil, api, map[string]string{
			"username":  ugn.Username,
			"groupname": ugn.GroupName,
			"namespace": ugn.Namespace,
		}, notify)
		if resp.Code != sdkconst.Success {
			sdklog.Logger.Errorf("TaskCenter publish progress err: %v", resp.Msg)
		}
	}
}

func (t *TaskCenter) HandleInterruptTask() {
}
