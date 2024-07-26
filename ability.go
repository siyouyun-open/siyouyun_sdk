package siyouyunsdk

import (
	"encoding/json"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/siyouyun-open/siyouyun_sdk/ability"
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
	fs       *ability.FS       // fs file handler
	milvus   *ability.Milvus   // milvus db
}

// WithFS add fs support
func (a *AppStruct) WithFS() {
	a.Ability.fs = ability.NewFS(&a.AppCode, a.db)
	log.Printf("[INFO] [%v] ability is supported", a.Ability.fs.Name())
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

// WithSchedule add schedule support
func (a *AppStruct) WithSchedule() {
	a.Ability.schedule = ability.NewSchedule(&a.AppCode)
	log.Printf("[INFO] [%v] ability is supported", a.Ability.schedule.Name())
	//启动监听event
	if a.nc == nil {
		return
	}
	go func() {
		_, err := a.nc.Subscribe(a.AppCode+"_schedule", func(msg *nats.Msg) {
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
				h.Handler(&se)
			}
			return
		})
		if err != nil {
			log.Printf("[ERROR] WithSchedule subscribe err: %v", err)
		}
	}()
}

// WithMessage add message support
func (a *AppStruct) WithMessage() {
	a.Ability.message = ability.NewMessage(&a.AppCode, a.nc)
	log.Printf("[INFO] [%v] ability is supported", a.Ability.message.Name())
}

// WithAI add AI support
func (a *AppStruct) WithAI() {
	a.Ability.ai = ability.NewAI()
	log.Printf("[INFO] [%v] ability is supported", a.Ability.ai.Name())
}

// WithMilvus add milvus support
func (a *AppStruct) WithMilvus() {
	var err error
	a.Ability.milvus, err = ability.NewMilvus(&a.AppCode)
	if err != nil {
		log.Printf("[ERROR] [%v] ability enable err: %v", a.Ability.milvus.Name(), err)
		return
	}
	log.Printf("[INFO] [%v] ability is supported", a.Ability.milvus.Name())
}

func (a *Ability) FS() *ability.FS {
	return a.fs
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
	// conn not ready, retry
	if a.ai.AIServiceClient == nil {
		a.ai = ability.NewAI()
		// check again
		if a.ai.AIServiceClient == nil {
			return nil, errors.New("ai service conn err")
		}
	}
	return a.ai, nil
}

func (a *Ability) Milvus() (*ability.Milvus, error) {
	if a.milvus == nil {
		return nil, abilityNotEnableErr
	}
	return a.milvus, nil
}

func (a *Ability) Destroy() {
	if a.kv != nil {
		a.kv.Close()
	}
	if a.ffmpeg != nil {
		a.ffmpeg.Close()
	}
	if a.schedule != nil {
		a.schedule.Close()
	}
	if a.message != nil {
		a.message.Close()
	}
	if a.ai != nil {
		a.ai.Close()
	}
	if a.milvus != nil {
		a.milvus.Close()
	}
}
