package gateway

import (
	"errors"
	"fmt"
	"github.com/robfig/cron"
	"github.com/siyouyun-open/siyouyun_sdk/const"
	"github.com/siyouyun-open/siyouyun_sdk/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
	"strconv"
	"time"
)

const (
	ScheduleOnceCreateApi = "/once/create"
	ScheduleOnceUpdateApi = "/once/update"
	ScheduleCronCreateApi = "/cron/create"
	ScheduleCronUpdateApi = "/cron/update"
)

type OnceCreateBody struct {
	AppCode    string `json:"appCode"`
	Username   string `json:"username"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Payload    []byte `json:"payload"`
	RemindTime int64  `json:"remindTime"`
}

type CronCreateBody struct {
	AppCode   string `json:"appCode"`
	Username  string `json:"username"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Payload   []byte `json:"payload"`
	Cron      string `json:"cron"`
}

type ScheduleApi struct {
	Host    string
	AppCode string
	*utils.UserNamespace
}

var scheduleGatewayAddr = fmt.Sprintf("%s:%d/%s", LocalhostAddress, CoreHTTPPort, "schedule")

func NewScheduleApi(appCode string, un *utils.UserNamespace) *ScheduleApi {
	return &ScheduleApi{
		Host:          scheduleGatewayAddr,
		AppCode:       appCode,
		UserNamespace: un,
	}
}

func (sa *ScheduleApi) OnceCreate(name string, payload []byte, remindTime int64) (error, *int64) {
	if time.Now().UnixMilli() > remindTime {
		return errors.New("remind time error"), nil
	}
	api := sa.Host + ScheduleOnceCreateApi
	response := restclient.PostRequest[int64](
		sa.UserNamespace,
		api,
		nil,
		OnceCreateBody{
			AppCode:    sa.AppCode,
			Username:   sa.Username,
			Namespace:  sa.Namespace,
			Name:       name,
			Payload:    payload,
			RemindTime: remindTime,
		},
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg), nil
	}
	return nil, response.Data
}

func (sa *ScheduleApi) OnceUpdate(eventId int64, remindTime int64) error {
	if time.Now().UnixMilli() > remindTime {
		return errors.New("remind time error")
	}
	api := sa.Host + ScheduleOnceUpdateApi
	response := restclient.PostRequest[any](
		sa.UserNamespace,
		api,
		map[string]string{
			"eventId":    strconv.FormatInt(eventId, 10),
			"remindTime": strconv.FormatInt(remindTime, 10),
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

func (sa *ScheduleApi) CronCreate(name string, payload []byte, c string) (error, *int64) {
	api := sa.Host + ScheduleCronCreateApi
	var response = restclient.PostRequest[int64](
		sa.UserNamespace,
		api,
		nil,
		CronCreateBody{
			AppCode:   sa.AppCode,
			Username:  sa.Username,
			Namespace: sa.Namespace,
			Name:      name,
			Payload:   payload,
			Cron:      c,
		},
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg), nil
	}
	return nil, nil
}

func (sa *ScheduleApi) CronUpdate(eventId int64, c string) error {
	api := sa.Host + ScheduleCronUpdateApi
	var response = restclient.PostRequest[int](
		sa.UserNamespace,
		api,
		map[string]string{
			"eventId": strconv.FormatInt(eventId, 10),
			"cron":    c,
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

func checkCron(c string) bool {
	s, err := cron.Parse(c)
	t1 := s.Next(time.Now())
	t2 := s.Next(t1)
	// fixme : simple check duration
	if t2.UnixMilli()-t1.UnixMilli() < 60*1e3 {
		return false
	}
	return err == nil
}
