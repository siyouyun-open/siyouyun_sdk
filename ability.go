package siyouyunsdk

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"

	"github.com/siyouyun-open/siyouyun_sdk/v2/ability"
	"github.com/siyouyun-open/siyouyun_sdk/v2/internal/transport"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/log"
)

var abilityNotEnableErr = errors.New("this ability not enabled yet")

// AbilityInterface defines the interface for an ability.
type AbilityInterface interface {
	Name() string
	IsReady() bool
	Close()
}

// Ability holds all optional app abilities.
type Ability struct {
	fs       *ability.FS
	kv       *ability.KV
	ffmpeg   *ability.FFmpeg
	schedule *ability.Schedule
	message  *ability.Message
	ai       *ability.AI
	fem      *ability.FileEventMonitor
	sem      *ability.SystemEventMonitor
	task     *ability.TaskCenter
	device   *ability.Device
}

// InitAbility initializes default abilities (FS is always enabled).
func (a *AppStruct) InitAbility() {
	a.Ability = &Ability{
		fs: ability.NewFS(&a.AppCode, a.db),
	}
}

// WithFS enables file system ability.
func (a *AppStruct) WithFS() {
	a.Ability.fs = ability.NewFS(&a.AppCode, a.db)
	sdklog.Logger.Infof("[%v] ability is supported", a.Ability.fs.Name())
}

// WithKV enables key-value store ability.
func (a *AppStruct) WithKV() {
	a.Ability.kv = ability.NewKV(&a.AppCode)
	sdklog.Logger.Infof("[%v] ability is supported", a.Ability.kv.Name())
}

// WithFFmpeg enables FFmpeg media processing ability.
func (a *AppStruct) WithFFmpeg() {
	a.Ability.ffmpeg = ability.NewFFmpeg()
	sdklog.Logger.Infof("[%v] ability is supported", a.Ability.ffmpeg.Name())
}

// WithSchedule enables schedule/reminder ability.
// In RUNC mode, schedule triggers are received via bus pub/sub.
func (a *AppStruct) WithSchedule() {
	a.Ability.schedule = ability.NewSchedule(&a.AppCode)
	sdklog.Logger.Infof("[%v] ability is supported", a.Ability.schedule.Name())

	if a.isRUNC {
		topic := fmt.Sprintf("system.schedule.%s", a.AppCode)
		bus := transport.GetBus()
		if bus != nil {
			bus.SubscribeTopic(topic)
			bus.OnTopicEvent(topic, func(topic string, event json.RawMessage) {
				var se ability.ScheduleEvent
				if err := json.Unmarshal(event, &se); err != nil {
					return
				}
				if h, ok := a.Ability.schedule.Handler[se.Name]; ok {
					h.Handler(&se)
				}
			})
		}
		return
	}

	// host mode: NATS listener
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
		})
		if err != nil {
			sdklog.Logger.Errorf("WithSchedule subscribe err: %v", err)
		}
	}()
}

// WithMessage enables message bot ability.
func (a *AppStruct) WithMessage() {
	if a.isRUNC {
		a.Ability.message = ability.NewMessageBus(&a.AppCode)
		sdklog.Logger.Infof("[%v] ability is supported (bus mode)", a.Ability.message.Name())
		return
	}
	a.Ability.message = ability.NewMessage(&a.AppCode, a.nc)
	sdklog.Logger.Infof("[%v] ability is supported", a.Ability.message.Name())
}

// WithAI enables AI inference ability (gRPC, host mode only).
// In RUNC mode this is skipped; a bus-based replacement is planned.
func (a *AppStruct) WithAI() {
	if a.isRUNC {
		sdklog.Logger.Infof("[AI] skipped in RUNC mode (will be replaced by bus-based AI)")
		return
	}
	a.Ability.ai = ability.NewAI()
	sdklog.Logger.Infof("[%v] ability is supported", a.Ability.ai.Name())
}

// WithFileEventMonitor enables file event monitoring.
func (a *AppStruct) WithFileEventMonitor(preferOps ...sdkdto.PreferOptions) {
	if len(preferOps) == 0 {
		return
	}
	a.Ability.fem = ability.NewFileEventMonitor(&a.AppCode, a.nc, preferOps...)
	sdklog.Logger.Infof("[%v] ability is supported", a.Ability.fem.Name())
}

// WithSystemEventMonitor enables system event monitoring.
// In RUNC mode, uses bus pub/sub instead of NATS JetStream.
func (a *AppStruct) WithSystemEventMonitor(opts ...ability.SystemEventOption) {
	if len(opts) == 0 {
		return
	}
	a.Ability.sem = ability.NewSystemEventMonitor(a.Ability.fs, &a.Ability.kv, a.nc, a.appInfo, &a.dataVersion, opts...)
	sdklog.Logger.Infof("[%v] ability is supported", a.Ability.sem.Name())
}

// WithDevice enables device info ability.
func (a *AppStruct) WithDevice() {
	a.Ability.device = ability.NewDevice()
	sdklog.Logger.Infof("[%v] ability is supported", a.Ability.device.Name())
}

// WithTaskCenter enables task center ability.
// In RUNC mode, uses BusBroker instead of NATS.
func (a *AppStruct) WithTaskCenter() {
	a.Ability.task = ability.NewTaskCenter(&a.AppCode, a.nc)
	sdklog.Logger.Infof("[%v] ability is supported", a.Ability.task.Name())
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
	if a.ai.AIServiceClient == nil {
		a.ai = ability.NewAI()
		if a.ai.AIServiceClient == nil {
			return nil, errors.New("ai service conn err")
		}
	}
	return a.ai, nil
}

func (a *Ability) FileEventMonitor() (*ability.FileEventMonitor, error) {
	if a.fem == nil {
		return nil, abilityNotEnableErr
	}
	return a.fem, nil
}

func (a *Ability) Device() (*ability.Device, error) {
	if a.device == nil {
		return nil, abilityNotEnableErr
	}
	return a.device, nil
}

func (a *Ability) TaskCenter() (*ability.TaskCenter, error) {
	if a.task == nil {
		return nil, abilityNotEnableErr
	}
	return a.task, nil
}

func (a *Ability) Destroy() {
	if a.fs != nil {
		a.fs.Close()
	}
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
	if a.fem != nil {
		a.fem.Close()
	}
	if a.sem != nil {
		a.sem.Close()
	}
	if a.task != nil {
		a.task.Close()
	}
	if a.device != nil {
		a.device.Close()
	}
}
