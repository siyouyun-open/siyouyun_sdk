package ability

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
	"log"
	"time"
)

const (
	jsName             = "SYY_ASYNC_STREAM"
	jsConsumerTemplate = "faaSConsumer_%s"
	jsEventPipe        = "async.faas.%s"
	faaSMigrationEvent = "faas_migration" // faas migration event
)

type Migrator struct {
	fs      *FS
	appInfo *sdkdto.AppRegisterInfo
	nc      *nats.Conn
	handler IMigrator
}

type IMigrator interface {
	// MigrateSchema migrate schema when start migrator and user registered
	MigrateSchema(ugn *utils.UserGroupNamespace) error
	// MigrateData migrate data when user registered
	MigrateData(ugn *utils.UserGroupNamespace) error
}

func NewMigrator(fs *FS, appInfo *sdkdto.AppRegisterInfo, nc *nats.Conn, handler IMigrator) *Migrator {
	m := &Migrator{
		fs:      fs,
		appInfo: appInfo,
		nc:      nc,
		handler: handler,
	}
	// start all ugn schema when first migration
	for i := range appInfo.UGNList {
		if err := handler.MigrateSchema(&appInfo.UGNList[i]); err != nil {
			log.Printf("[ERROR] NewMigrator first migration err: %v", err)
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
			log.Printf("[ERROR] Migrator listen err: %v", err)
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
	case faaSMigrationEvent:
		var ugn utils.UserGroupNamespace
		err = json.Unmarshal(event.Payload, &ugn)
		if err != nil {
			log.Printf("[ERROR] handleEvent parse err: %v", err)
			return
		}
		log.Printf("[INFO] migration event, ugn: %+v", ugn)
		m.migrateWithUser(&ugn)
	default:
		log.Printf("[WARN] undefined message type: %s", event.EventName)
	}
}

func (m *Migrator) migrateWithUser(ugn *utils.UserGroupNamespace) {
	// migrate schema
	err := m.handler.MigrateSchema(ugn)
	if err != nil {
		log.Printf("[ERROR] migrateWithUser migrate schema err: %v", err)
		return
	}
	// migrate data
	err = m.handler.MigrateData(ugn)
	if err != nil {
		log.Printf("[ERROR] migrateWithUser migrate data err: %v", err)
		return
	}
}

// processingEvent async event struct
type processingEvent struct {
	EventName string `json:"eventName"`
	Payload   []byte `json:"payload"`
}
