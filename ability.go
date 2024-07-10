package siyouyunsdk

import (
	"encoding/json"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/internal/ability"
	"log"
)

var abilityNotEnableErr = errors.New("this ability not enabled yet")

// AbilityInterface ability interface
type AbilityInterface interface {
	// Name of ability
	Name() string
	// Close resources
	Close()
}

// Ability app ability set
type Ability struct {
	kv       *ability.KV       // kv store
	ffmpeg   *ability.FFmpeg   // ffmpeg info
	schedule *ability.Schedule // schedule remind
	message  *ability.Message  // message bot
	ai       *ability.AI       // ai inference
}

// WithKV add kv support
func (a *AppStruct) WithKV() {
	a.Ability.kv = ability.NewKV(&a.AppCode)
	log.Printf("[INFO] [%v] ability is supported", a.Ability.kv.Name())
}

// WithFFmpeg add ffmpeg support
func (a *AppStruct) WithFFmpeg() {
	a.Ability.ffmpeg = ability.NewFFmpeg()
	log.Printf("[INFO] [%v] ability is supported", a.Ability.ffmpeg.Name())
}

// WithScheduleEvent add schedule support
func (a *AppStruct) WithScheduleEvent() {
	a.Ability.schedule = ability.NewSchedule(&a.AppCode)
	log.Printf("[INFO] [%v] ability is supported", a.Ability.schedule.Name())
	//启动监听event
	if a.nc == nil {
		return
	}
	go func() {
		_, _ = a.nc.Subscribe(a.AppCode+"_schedule", func(msg *nats.Msg) {
			var se ability.ScheduleEvent
			defer func() {
				if err := recover(); err != nil {
					return
				}
			}()
			err := json.Unmarshal(msg.Data, &se)
			if err != nil {
				return
			}
			if h, ok := a.Ability.schedule.Handler[se.Name]; !ok {
				return
			} else {
				eventfs := a.newEventFSFromScheduleEvent(&se)
				h.Handler(eventfs, &se)
				eventfs.Destroy()
			}
			return
		})
	}()
}

// WithMessage add message support
func (a *AppStruct) WithMessage() {
	a.Ability.message = ability.NewMessage(a.nc)
	log.Printf("[INFO] [%v] ability is supported", a.Ability.message.Name())
}

// WithAI add AI support
func (a *AppStruct) WithAI() {
	a.Ability.ai = ability.NewAI()
	log.Printf("[INFO] [%v] ability is supported", a.Ability.ai.Name())
}

func (a *Ability) KV() (*ability.KV, error) {
	if a.kv == nil {
		return nil, abilityNotEnableErr
	}
	return a.kv, nil
}

func (a *Ability) FFmpeg() (*ability.FFmpeg, error) {
	if a.ffmpeg == nil {
		return nil, abilityNotEnableErr
	}
	return a.ffmpeg, nil
}

func (a *Ability) Schedule() (*ability.Schedule, error) {
	if a.schedule == nil {
		return nil, abilityNotEnableErr
	}
	return a.schedule, nil
}

func (a *Ability) Message() (*ability.Message, error) {
	if a.message == nil {
		return nil, abilityNotEnableErr
	}
	return a.message, nil
}

func (a *Ability) AI() (*ability.AI, error) {
	if a.ai == nil {
		return nil, abilityNotEnableErr
	}
	return a.ai, nil
}

func (a *Ability) Destroy() {
	a.kv.Close()
	a.ffmpeg.Close()
	a.schedule.Close()
	a.message.Close()
	a.ai.Close()
}
