package ability

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"time"
)

const (
	jsName             = "SYY_ASYNC_STREAM"
	jsConsumerTemplate = "appConsumer_%s"
	jsEventPipe        = "async.app.%s"
	appMigrationEvent  = "app_migration" // app migration event
)

type Migrator struct {
	fs      *FS
	appInfo *sdkdto.AppRegisterInfo
	nc      *nats.Conn
	handler IMigrator
}

type IMigrator interface {
	// Migrate everything you want, schema or data
	Migrate(ugn *utils.UserGroupNamespace) error
}

func NewMigrator(fs *FS, appInfo *sdkdto.AppRegisterInfo, nc *nats.Conn, handler IMigrator) *Migrator {
	m := &Migrator{
		fs:      fs,
		appInfo: appInfo,
		nc:      nc,
		handler: handler,
	}
	// migrate all ugn when app startup
	for i := range appInfo.UGNList {
		if err := handler.Migrate(&appInfo.UGNList[i]); err != nil {
			sdklog.Logger.Errorf("NewMigrator first migration err: %v", err)
			break
		}
	}
	// start listener
	m.listen()
	return m
}

func (m *Migrator) Name() string {
	return "Migrator"
}

func (m *Migrator) Close() {
}

func (m *Migrator) listen() {
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

// handleEvent event handler
func (m *Migrator) handleEvent(msg jetstream.Msg) {
	var err error
	defer func() {
		if err == nil {
			_ = msg.Ack()
		} else {
			_ = msg.NakWithDelay(5 * time.Second)
		}
	}()
	var event processingEvent
	_ = json.Unmarshal(msg.Data(), &event)
	switch event.EventName {
	// migration event
	case appMigrationEvent:
		var ugn utils.UserGroupNamespace
		err = json.Unmarshal(event.Payload, &ugn)
		if err != nil {
			sdklog.Logger.Errorf("handleEvent parse err: %v", err)
			return
		}
		sdklog.Logger.Infof("migration event, ugn: %+v", ugn)
		m.migrateWithUser(&ugn)
	default:
		sdklog.Logger.Warnf("undefined message type: %s", event.EventName)
	}
}

func (m *Migrator) migrateWithUser(ugn *utils.UserGroupNamespace) {
	err := m.handler.Migrate(ugn)
	if err != nil {
		sdklog.Logger.Errorf("migrateWithUser migrate err: %v", err)
		return
	}
}

// processingEvent async event struct
type processingEvent struct {
	EventName string `json:"eventName"`
	Payload   []byte `json:"payload"`
}
