package siyouyunsdk

import (
	"encoding/json"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/gateway"
	"github.com/siyouyun-open/siyouyun_sdk/utils"
)

type Schedule struct {
	*gateway.ScheduleApi
}

func (fs *FS) NewSchedule() *Schedule {
	return &Schedule{
		ScheduleApi: gateway.NewScheduleApi(fs.AppCodeName, fs.UGN),
	}
}

func (s *Schedule) AddOnceScheduleEvent(name string, payload []byte, remindTime int64) (err error, eventId *int64) {
	return s.ScheduleApi.OnceCreate(name, payload, remindTime)
}

func (s *Schedule) UpdateOnceScheduleEvent(eventId int64, remindTime int64) (err error) {
	return s.ScheduleApi.OnceUpdate(eventId, remindTime)
}

func (s *Schedule) AddCronScheduleEvent(name string, payload []byte, c string) (err error, eventId *int64) {
	return s.ScheduleApi.CronCreate(name, payload, c)
}

func (s *Schedule) UpdateCronScheduleEvent(eventId int64, c string) (err error) {
	return s.ScheduleApi.CronUpdate(eventId, c)
}

type ScheduleEvent struct {
	UGN        utils.UserGroupNamespace
	RemindTime int64  `json:"remindTime"`
	Name       string `json:"name"`
	Payload    []byte `json:"payload"`
}

type ScheduleHandler struct {
	app     *AppStruct
	handler map[string]ScheduleEventHandler
}

type ScheduleEventHandler struct {
	Name    string
	Handler func(fs *EventFS, se *ScheduleEvent)
}

func (a *AppStruct) WithScheduleEvent() {
	a.Schedule = &ScheduleHandler{
		app:     a,
		handler: make(map[string]ScheduleEventHandler),
	}
}

func (sh *ScheduleHandler) SetHandler(shs ...ScheduleEventHandler) {
	for i := range shs {
		sh.handler[shs[i].Name] = shs[i]
	}
}

func (sh *ScheduleHandler) Listen() {
	if len(sh.handler) == 0 {
		return
	}
	//启动监听event
	nc := getNats()
	if nc == nil {
		return
	}
	go func() {
		_, _ = nc.Subscribe(sh.app.AppCode+"_schedule", func(msg *nats.Msg) {
			var se ScheduleEvent
			defer func() {
				if err := recover(); err != nil {
					return
				}
			}()
			err := json.Unmarshal(msg.Data, &se)
			if err != nil {
				return
			}
			if h, ok := sh.handler[se.Name]; !ok {
				return
			} else {
				eventfs := sh.app.newEventFSFromScheduleEvent(&se)
				h.Handler(eventfs, &se)
				eventfs.Destroy()
			}

			return
		})
	}()
}
