package taskcenter

import (
	"time"

	"github.com/nats-io/nats.go"
)

type natsBrokerWrapper struct {
	conn *nats.Conn
}

func NewNATSBroker(conn *nats.Conn) MessageBroker {
	return &natsBrokerWrapper{conn: conn}
}

func (w *natsBrokerWrapper) Publish(subject string, data []byte) error {
	return w.conn.Publish(subject, data)
}

func (w *natsBrokerWrapper) Subscribe(subject string, handler MsgHandler) (Subscription, error) {
	sub, err := w.conn.Subscribe(subject, func(msg *nats.Msg) {
		handler(&Msg{
			Subject: msg.Subject,
			Data:    msg.Data,
			respond: func(data []byte) error { return msg.Respond(data) },
		})
	})
	if err != nil {
		return nil, err
	}
	return &natsSubWrapper{sub: sub}, nil
}

func (w *natsBrokerWrapper) Request(subject string, data []byte, timeout time.Duration) (*Msg, error) {
	msg, err := w.conn.Request(subject, data, timeout)
	if err != nil {
		return nil, err
	}
	return &Msg{
		Subject: msg.Subject,
		Data:    msg.Data,
	}, nil
}

type natsSubWrapper struct {
	sub *nats.Subscription
}

func (w *natsSubWrapper) Unsubscribe() error {
	return w.sub.Unsubscribe()
}
