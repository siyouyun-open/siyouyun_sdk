package capability

import (
	"context"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// Daemon is the read-only capability of the device-side daemon process.
type Daemon struct {
	client *bus.Client
}

// NewDaemon creates a Daemon wrapper.
func NewDaemon(client *bus.Client) *Daemon { return &Daemon{client: client} }

// Health reports whether the daemon is online. It is the lightest-weight probe of "is the bus still
// reachable" and is suitable for health checks.
func (d *Daemon) Health(ctx context.Context) (bool, error) {
	var res struct {
		Online bool `json:"online"`
	}
	if err := d.client.CallInto(ctx, &bus.Request{
		Template: protocol.TemplateDaemonHealth,
	}, &res); err != nil {
		return false, err
	}
	return res.Online, nil
}

// Call is a pass-through call to any template, for platform capabilities and custom capabilities
// that do not yet have a strongly typed wrapper.
//
// Built-in templates do not need a signature; custom.* must supply the exact signature obtained from
// capability discovery.
func (c *Client) Call(ctx context.Context, req *bus.Request, out any) error {
	return c.client.CallInto(ctx, req, out)
}

// Client binds the bus client together with the capability domains.
type Client struct {
	client   *bus.Client
	kv       *KV
	daemon   *Daemon
	media    *Media
	device   *Device
	storage  *Storage
	message  *Message
	schedule *Schedule
	events   *Events
}

// New creates the capability facade. When busClient is nil, calls on the returned facade fail (they
// do not panic).
func New(busClient *bus.Client) *Client {
	return &Client{
		client:   busClient,
		kv:       NewKV(busClient),
		daemon:   NewDaemon(busClient),
		media:    NewMedia(busClient),
		device:   NewDevice(busClient),
		storage:  NewStorage(busClient),
		message:  NewMessage(busClient),
		schedule: NewSchedule(busClient),
		events:   NewEvents(busClient),
	}
}

// KV returns the KV capability.
func (c *Client) KV() *KV { return c.kv }

// Daemon returns the daemon capability.
func (c *Client) Daemon() *Daemon { return c.daemon }

// Media returns the media metadata capability.
func (c *Client) Media() *Media { return c.media }

// Device returns the device info capability.
func (c *Client) Device() *Device { return c.device }

// Storage returns the storage control-plane capability (space enumeration, trash).
func (c *Client) Storage() *Storage { return c.storage }

// Message returns the bot message capability.
func (c *Client) Message() *Message { return c.message }

// Events returns the file event capability (subscribe + watched-scope read/write).
func (c *Client) Events() *Events { return c.events }

// Schedule returns the scheduled task capability.
func (c *Client) Schedule() *Schedule { return c.schedule }

// Bus returns the underlying bus client.
func (c *Client) Bus() *bus.Client { return c.client }

// Subscribe subscribes to event topics.
func (c *Client) Subscribe(ctx context.Context, topics ...string) error {
	return c.client.Subscribe(ctx, topics...)
}

// SubscribeFileEvents subscribes to file events.
func (c *Client) SubscribeFileEvents(ctx context.Context, handler func(FileEvent)) (*Subscription, error) {
	return SubscribeFileEvents(ctx, c.client, handler)
}
