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
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"golang.org/x/sync/errgroup"
)

const (
	jsName             = "SYY_ASYNC_STREAM"
	jsConsumerTemplate = "appConsumer_%s"
	jsEventPipe        = "async.app.%s"
	appMigrationEvent  = "app_migration" // app migration event
	appCleanupEvent    = "app_cleanup"   // app cleanup event
)

type SystemEventMonitor struct {
	fs          *FS
	kv          **KV
	nc          *nats.Conn
	appInfo     *sdkdto.AppRegisterInfo
	dataVersion *int
	handlerMap  map[string]func(payload []byte) error
}

type SystemEventOption func(monitor *SystemEventMonitor)

func NewSystemEventMonitor(fs *FS, kv **KV, nc *nats.Conn,
	appInfo *sdkdto.AppRegisterInfo, dataVersion *int, opts ...SystemEventOption) *SystemEventMonitor {
	m := &SystemEventMonitor{
		fs:          fs,
		kv:          kv,
		nc:          nc,
		appInfo:     appInfo,
		dataVersion: dataVersion,
		handlerMap:  make(map[string]func(payload []byte) error),
	}
	// apply all options
	for _, opt := range opts {
		opt(m)
	}
	// start listener
	m.listen()
	return m
}

type IMigrator interface {
	// Migrate everything you want, schema or data
	Migrate(ugn *utils.UserGroupNamespace) error
}

type ICleanup interface {
	Cleanup(ugn *utils.UserGroupNamespace) error
}

// WithMigrationOption handle migration event
func WithMigrationOption(migrator IMigrator) SystemEventOption {
	return func(m *SystemEventMonitor) {
		m.handlerMap[appMigrationEvent] = func(payload []byte) error {
			var ugn utils.UserGroupNamespace
			err := json.Unmarshal(payload, &ugn)
			if err != nil {
				return err
			}
			err = migrator.Migrate(&ugn)
			if err != nil {
				return err
			}
			// If a data version is set, the initial version of the data will be automatically written
			if m.dataVersion != nil && *m.dataVersion > 0 && m.kv != nil && *m.kv != nil {
				err = (*m.kv).PutKV(&ugn, sdkconst.DefaultAppKeyType, sdkconst.AppDataVersionKey, strconv.Itoa(*m.dataVersion))
				if err != nil {
					sdklog.Logger.Errorf("migrate dataVersion err: %v", err)
				}
			}
			return nil
		}
		// migrate all ugn when invoked
		var eg errgroup.Group
		eg.SetLimit(4)
		for i := range m.appInfo.UGNList {
			j := i
			eg.Go(func() error {
				return migrator.Migrate(&m.appInfo.UGNList[j])
			})
		}
		if err := eg.Wait(); err != nil {
			sdklog.Logger.Errorf("WithMigrationHandler first migration err: %v", err)
		}
	}
}

// WithCleanupOption handle cleanup event
func WithCleanupOption(cleanup ICleanup) SystemEventOption {
	return func(m *SystemEventMonitor) {
		m.handlerMap[appCleanupEvent] = func(payload []byte) error {
			var ugn utils.UserGroupNamespace
			err := json.Unmarshal(payload, &ugn)
			if err != nil {
				return err
			}
			return cleanup.Cleanup(&ugn)
		}
	}
}

func (m *SystemEventMonitor) Name() string {
	return "SystemEventMonitor"
}

func (m *SystemEventMonitor) IsReady() bool {
	return true
}

func (m *SystemEventMonitor) Close() {
}

func (m *SystemEventMonitor) listen() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Create event pull consumer
	consumerCfg := jetstream.ConsumerConfig{
		Durable:       fmt.Sprintf(jsConsumerTemplate, m.appInfo.AppCode),
		FilterSubject: fmt.Sprintf(jsEventPipe, m.appInfo.AppCode),
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       1 * time.Minute,
		MaxDeliver:    10,
		MaxAckPending: 20,
	}
	js, _ := jetstream.New(m.nc)
	consumer, err := js.CreateOrUpdateConsumer(ctx, jsName, consumerCfg)
	if err != nil {
		_ = js.DeleteConsumer(ctx, jsName, consumerCfg.Durable)
		consumer, err = js.CreateConsumer(ctx, jsName, consumerCfg)
		if err != nil {
			sdklog.Logger.Errorf("Migrator listen err: %v", err)
			return
		}
	}
	consumer.Consume(m.handleEvent)
}

func (m *SystemEventMonitor) handleEvent(msg jetstream.Msg) {
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
	var event processingEvent
	_ = json.Unmarshal(msg.Data(), &event)
	handler, ok := m.handlerMap[event.EventName]
	if !ok {
		sdklog.Logger.Errorf("SystemEventMonitor event no handler: %v", event.EventName)
		return
	}
	err = handler(event.Payload)
	if err != nil {
		sdklog.Logger.Errorf("SystemEventMonitor handle err: %v", err)
		return
	}
}

// processingEvent async event struct
type processingEvent struct {
	EventName string `json:"eventName"`
	Payload   []byte `json:"payload"`
}
