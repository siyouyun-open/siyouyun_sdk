package ability

import (
	"errors"
	"strconv"
	"time"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

type Schedule struct {
	Handler     map[string]ScheduleEventHandler
	gatewayAddr string
	appCode     *string
}

type ScheduleEventHandler struct {
	Name    string
	Handler func(se *ScheduleEvent)
}

type ScheduleEvent struct {
	UGN        utils.UserGroupNamespace `json:"ugn"`
	RemindTime int64                    `json:"remindTime"`
	Name       string                   `json:"name"`
	Payload    []byte                   `json:"payload"`
}

func NewSchedule(appCode *string) *Schedule {
	return &Schedule{
		Handler:     make(map[string]ScheduleEventHandler),
		gatewayAddr: utils.GetCoreServiceURL() + "/v2/app/schedule",
		appCode:     appCode,
	}
}

func (s *Schedule) Name() string {
	return "Schedule"
}

func (s *Schedule) IsReady() bool {
	return isCoreServiceReady()
}

func (s *Schedule) Close() {
}

func (s *Schedule) SetHandler(shs ...ScheduleEventHandler) {
	for i := range shs {
		s.Handler[shs[i].Name] = shs[i]
	}
}

func (s *Schedule) AddOnceScheduleEvent(ugn *utils.UserGroupNamespace, name string, payload []byte, remindTime int64) (err error, eventId *int64) {
	if time.Now().UnixMilli() > remindTime {
		return errors.New("remind time error"), nil
	}
	api := s.gatewayAddr + "/once/create"
	response := restclient.PostRequest[int64](
		ugn,
		api,
		nil,
		onceCreateRequest{
			UGN:        ugn,
			AppCode:    *s.appCode,
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

func (s *Schedule) UpdateOnceScheduleEvent(ugn *utils.UserGroupNamespace, eventId int64, remindTime int64) (err error) {
	if time.Now().UnixMilli() > remindTime {
		return errors.New("remind time error")
	}
	api := s.gatewayAddr + "/once/update"
	response := restclient.PostRequest[any](
		ugn,
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

func (s *Schedule) AddCronScheduleEvent(ugn *utils.UserGroupNamespace, name string, payload []byte, cron string) (err error, eventId *int64) {
	api := s.gatewayAddr + "/cron/create"
	var response = restclient.PostRequest[int64](
		ugn,
		api,
		nil,
		cronCreateRequest{
			UGN:     ugn,
			AppCode: *s.appCode,
			Name:    name,
			Payload: payload,
			Cron:    cron,
		},
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg), nil
	}
	return nil, nil
}

func (s *Schedule) UpdateCronScheduleEvent(ugn *utils.UserGroupNamespace, eventId int64, cron string) (err error) {
	api := s.gatewayAddr + "/cron/update"
	var response = restclient.PostRequest[int](
		ugn,
		api,
		map[string]string{
			"eventId": strconv.FormatInt(eventId, 10),
			"cron":    cron,
		},
		nil,
	)
	if response.Code != sdkconst.Success {
		return errors.New(response.Msg)
	}
	return nil
}

type onceCreateRequest struct {
	UGN        *utils.UserGroupNamespace
	AppCode    string `json:"appCode"`
	Name       string `json:"name"`
	Payload    []byte `json:"payload"`
	RemindTime int64  `json:"remindTime"`
}

type cronCreateRequest struct {
	UGN     *utils.UserGroupNamespace
	AppCode string `json:"appCode"`
	Name    string `json:"name"`
	Payload []byte `json:"payload"`
	Cron    string `json:"cron"`
}
