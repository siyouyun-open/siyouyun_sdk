package ability

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"

	"github.com/siyouyun-open/siyouyun_sdk/v2/internal/transport"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/v2/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/v2/pkg/utils"
)

const (
	jsStreamName       = "SYY_ASYNC_STREAM"
	jsConsumerTemplate = "appConsumer_%s"
	jsEventSubject     = "async.app.%s"
	busSystemTopic     = "system.event.%s"
	appMigrationEvent  = "app_migration"
	appCleanupEvent    = "app_cleanup"
)

type SystemEventMonitor struct {
	fs          *FS
	kv          **KV
	nc          *nats.Conn
	bus         *transport.BusTransport
	appInfo     *sdkdto.AppRegisterInfo
	dataVersion *int
	handlerMap  map[string]func(payload []byte) error
}

type SystemEventOption func(monitor *SystemEventMonitor)

// NewSystemEventMonitor creates a system event monitor.
// In RUNC mode, uses bus pub/sub instead of NATS JetStream.
func NewSystemEventMonitor(fs *FS, kv **KV, nc *nats.Conn,
	appInfo *sdkdto.AppRegisterInfo, dataVersion *int, opts ...SystemEventOption) *SystemEventMonitor {

	m := &SystemEventMonitor{
		fs:          fs,
		kv:          kv,
		nc:          nc,
		bus:         transport.GetBus(),
		appInfo:     appInfo,
		dataVersion: dataVersion,
		handlerMap:  make(map[string]func(payload []byte) error),
	}
	for _, opt := range opts {
		opt(m)
	}

	if utils.IsRUNC() {
		m.listenBus()
	} else {
		m.listenNATS()
	}
	return m
}

// ── Options ──

type IMigrator interface {
	Migrate(ugn *utils.UserGroupNamespace) error
}

type ICleanup interface {
	Cleanup(ugn *utils.UserGroupNamespace) error
}

func WithMigrationOption(migrator IMigrator) SystemEventOption {
	return func(m *SystemEventMonitor) {
		m.handlerMap[appMigrationEvent] = func(payload []byte) error {
			var ugn utils.UserGroupNamespace
			if err := json.Unmarshal(payload, &ugn); err != nil {
				return err
			}
			if err := migrator.Migrate(&ugn); err != nil {
				return err
			}
			if m.dataVersion != nil && *m.dataVersion > 0 && m.kv != nil && *m.kv != nil {
				if err := (*m.kv).PutKV(&ugn, sdkconst.DefaultAppKeyType, sdkconst.AppDataVersionKey, strconv.Itoa(*m.dataVersion)); err != nil {
					sdklog.Logger.Errorf("migrate dataVersion err: %v", err)
				}
			}
			return nil
		}
		var eg errgroup.Group
		eg.SetLimit(4)
		for i := range m.appInfo.UGNList {
			j := i
			eg.Go(func() error { return migrator.Migrate(&m.appInfo.UGNList[j]) })
		}
		if err := eg.Wait(); err != nil {
			sdklog.Logger.Errorf("WithMigrationHandler first migration err: %v", err)
		}
	}
}

func WithCleanupOption(cleanup ICleanup) SystemEventOption {
	return func(m *SystemEventMonitor) {
		m.handlerMap[appCleanupEvent] = func(payload []byte) error {
			var ugn utils.UserGroupNamespace
			if err := json.Unmarshal(payload, &ugn); err != nil {
				return err
			}
			return cleanup.Cleanup(&ugn)
		}
	}
}

func (m *SystemEventMonitor) Name() string  { return "SystemEventMonitor" }
func (m *SystemEventMonitor) IsReady() bool { return true }
func (m *SystemEventMonitor) Close()        {}

// ── Bus transport (RUNC mode) ──

func (m *SystemEventMonitor) listenBus() {
	if m.bus == nil || len(m.handlerMap) == 0 {
		return
	}
	topic := fmt.Sprintf(busSystemTopic, m.appInfo.AppCode)
	m.bus.SubscribeTopic(topic)
	m.bus.OnTopicEvent(topic, func(topic string, event json.RawMessage) {
		var pe processingEvent
		if err := json.Unmarshal(event, &pe); err != nil {
			sdklog.Logger.Errorf("SystemEventMonitor unmarshal: %v", err)
			return
		}
		handler, ok := m.handlerMap[pe.EventName]
		if !ok {
			sdklog.Logger.Errorf("SystemEventMonitor no handler for: %s", pe.EventName)
			return
		}
		if err := handler(pe.Payload); err != nil {
			sdklog.Logger.Errorf("SystemEventMonitor handle err: %v", err)
		}
	})
}

// ── NATS/JetStream (host mode) ──

func (m *SystemEventMonitor) listenNATS() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	consumerCfg := jetstream.ConsumerConfig{
		Durable:       fmt.Sprintf(jsConsumerTemplate, m.appInfo.AppCode),
		FilterSubject: fmt.Sprintf(jsEventSubject, m.appInfo.AppCode),
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       1 * time.Minute,
		MaxDeliver:    10,
		MaxAckPending: 20,
	}
	js, _ := jetstream.New(m.nc)
	consumer, err := js.CreateOrUpdateConsumer(ctx, jsStreamName, consumerCfg)
	if err != nil {
		_ = js.DeleteConsumer(ctx, jsStreamName, consumerCfg.Durable)
		consumer, err = js.CreateConsumer(ctx, jsStreamName, consumerCfg)
		if err != nil {
			sdklog.Logger.Errorf("SystemEventMonitor listen err: %v", err)
			return
		}
	}
	consumer.Consume(m.handleJetStreamEvent)
}

func (m *SystemEventMonitor) handleJetStreamEvent(msg jetstream.Msg) {
	var err error
	defer func() {
		if r := recover(); r != nil {
			sdklog.Logger.Errorf("SystemEventMonitor handleEvent panic: %v\nStack trace:\n%s", r, debug.Stack())
			_ = msg.NakWithDelay(5 * time.Second)
			return
		}
		if err == nil {
			_ = msg.Ack()
		} else {
			_ = msg.NakWithDelay(5 * time.Second)
		}
	}()
	var pe processingEvent
	if err = json.Unmarshal(msg.Data(), &pe); err != nil {
		return
	}
	handler, ok := m.handlerMap[pe.EventName]
	if !ok {
		sdklog.Logger.Errorf("SystemEventMonitor event no handler: %v", pe.EventName)
		return
	}
	err = handler(pe.Payload)
	if err != nil {
		sdklog.Logger.Errorf("SystemEventMonitor handle err: %v", err)
	}
}

type processingEvent struct {
	EventName string `json:"eventName"`
	Payload   []byte `json:"payload"`
}
