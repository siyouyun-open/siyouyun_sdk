package siyouyunsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nats-io/nats.go/jetstream"
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

func (a *AppStruct) listenSysEvent() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Create event consumer，push subscribe
	consumerCfg := jetstream.ConsumerConfig{
		Durable:       fmt.Sprintf(jsConsumerTemplate, a.AppCode),
		FilterSubject: fmt.Sprintf(jsEventPipe, a.AppCode),
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       1 * time.Minute,
		MaxDeliver:    10,
		MaxAckPending: 20,
	}
	js, _ := jetstream.New(a.nc)
	consumer, err := js.CreateOrUpdateConsumer(ctx, jsName, consumerCfg)
	if err != nil {
		_ = js.DeleteConsumer(ctx, jsName, consumerCfg.Durable)
		consumer, err = js.CreateConsumer(ctx, jsName, consumerCfg)
		if err != nil {
			log.Printf("[ERROR] listenSysEvent err: %v", err)
			return
		}
	}
	_, _ = consumer.Consume(a.handleEvent)
}

// handleEvent event handler
func (a *AppStruct) handleEvent(msg jetstream.Msg) {
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
		a.migrateWithUser(&ugn)
	default:
		log.Printf("[WARN] Undefined message type: %s", event.EventName)
	}
}

// processingEvent async event struct
type processingEvent struct {
	EventName string `json:"eventName"`
	Payload   []byte `json:"payload"`
}
