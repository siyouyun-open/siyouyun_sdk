package capability

import (
	"context"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// Message is the app bot message capability.
type Message struct{ client *bus.Client }

// NewMessage creates a Message wrapper.
func NewMessage(client *bus.Client) *Message { return &Message{client: client} }

// Send sends a message to a user conversation as the app bot.
//
// A non-empty replyUUID means replying to a message; a type of 0 uses the upstream default.
func (m *Message) Send(ctx context.Context, scope Scope, content, replyUUID string) error {
	payload := scope.payload()
	payload["content"] = content
	if replyUUID != "" {
		payload["replyUUID"] = replyUUID
	}
	return callEnvelope(ctx, m.client, &bus.Request{
		Template: protocol.TemplateMessageSend,
		Payload:  payload,
	}, nil)
}

// Schedule is the scheduled task capability.
//
// When a task fires, the platform delivers an event by appCode; see protocol.TopicScheduleEvent for
// the consumer side.
type Schedule struct{ client *bus.Client }

// NewSchedule creates a Schedule wrapper.
func NewSchedule(client *bus.Client) *Schedule { return &Schedule{client: client} }

// OnceSpec is the definition of a one-shot task.
type OnceSpec struct {
	// Name is the task name (callbacks are dispatched by it).
	Name string
	// Payload is the string payload delivered with the event.
	Payload string
	// RemindTime is the trigger time (Unix milliseconds) and must be later than the current time.
	RemindTime int64
}

// CronSpec is the definition of a recurring task.
type CronSpec struct {
	Name    string
	Payload string
	Cron    string
}

// AddOnce creates a one-shot scheduled task and returns the event ID (used for later updates).
func (s *Schedule) AddOnce(ctx context.Context, scope Scope, spec OnceSpec) (int64, error) {
	payload := scope.payload()
	payload["name"] = spec.Name
	payload["payload"] = spec.Payload
	payload["remindTime"] = spec.RemindTime

	var eventID int64
	if err := callEnvelope(ctx, s.client, &bus.Request{
		Template: protocol.TemplateScheduleOnceCreate,
		Payload:  payload,
	}, &eventID); err != nil {
		return 0, err
	}
	return eventID, nil
}

// AddCron creates a recurring scheduled task and returns the event ID.
func (s *Schedule) AddCron(ctx context.Context, scope Scope, spec CronSpec) (int64, error) {
	payload := scope.payload()
	payload["name"] = spec.Name
	payload["payload"] = spec.Payload
	payload["cron"] = spec.Cron

	var eventID int64
	if err := callEnvelope(ctx, s.client, &bus.Request{
		Template: protocol.TemplateScheduleCronCreate,
		Payload:  payload,
	}, &eventID); err != nil {
		return 0, err
	}
	return eventID, nil
}

// UpdateOnce updates the trigger time of a one-shot task.
func (s *Schedule) UpdateOnce(ctx context.Context, scope Scope, eventID, remindTime int64) error {
	payload := scope.payload()
	payload["eventId"] = eventID
	payload["remindTime"] = remindTime
	return callEnvelope(ctx, s.client, &bus.Request{
		Template: protocol.TemplateScheduleOnceUpdate,
		Payload:  payload,
	}, nil)
}

// UpdateCron updates the cron expression of a recurring task.
func (s *Schedule) UpdateCron(ctx context.Context, scope Scope, eventID int64, cron string) error {
	payload := scope.payload()
	payload["eventId"] = eventID
	payload["cron"] = cron
	return callEnvelope(ctx, s.client, &bus.Request{
		Template: protocol.TemplateScheduleCronUpdate,
		Payload:  payload,
	}, nil)
}
