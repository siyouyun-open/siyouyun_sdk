package taskcenter

import "time"

// MessageBroker is the abstract message broker interface for communication between task centers.
// It supports publish-subscribe (progress push, status change) and request-response (operation routing) patterns.
// The default implementation uses NATS; for testing, it can be replaced with a MockBroker.
type MessageBroker interface {
	Publish(subject string, data []byte) error
	Subscribe(subject string, handler MsgHandler) (Subscription, error)
	Request(subject string, data []byte, timeout time.Duration) (*Msg, error)
}

// MsgHandler is the message processing callback function.
type MsgHandler func(msg *Msg)

// Subscription is a subscription handle used to unsubscribe.
type Subscription interface {
	Unsubscribe() error
}

// Msg is a message wrapper containing the subject, data, and response method.
type Msg struct {
	Subject string
	Data    []byte
	respond func(data []byte) error
}

func (m *Msg) Respond(data []byte) error {
	if m.respond != nil {
		return m.respond(data)
	}
	return nil
}
