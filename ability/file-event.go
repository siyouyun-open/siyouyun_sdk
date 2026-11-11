package ability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	sdklog "github.com/siyouyun-open/siyouyun_sdk/pkg/log"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/utils"
)

// FileEventMonitor implements the v1 SDK target-mode file-event listener:
//   - each PreferOptions requests one listener from the OS (listenerId + durable consumer);
//   - the SDK consumes the shared stream with pull Fetch;
//   - it only checks whether payload.matched.listeners contains its listenerId;
//   - on match it calls Handler and ACKs/NAKs according to ConsumeStatus;
//   - it no longer registers /app/event/register or subscribes to appCode_event.
type FileEventMonitor struct {
	appCode       *string
	nc            *nats.Conn
	js            jetstream.JetStream
	preferOptions map[string]sdkdto.PreferOptions
	listeners     map[string]*interestListener

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
}

type interestListener struct {
	id          string
	appCode     string
	consumer    string
	stream      string
	ttl         time.Duration
	handler     func(fe *sdkdto.FileEvent) (sdkconst.ConsumeStatus, string)
	workerLimit atomic.Int64
	controlSub  *nats.Subscription
	cons        jetstream.Consumer
	ctx         context.Context
	cancel      context.CancelFunc
}

// FileEventListenerOptions defines the filters for StartFileEventListener.
//
// It mirrors the old PreferOptions, but actions/media are lists and the SDK converts
// them into the OS interest dirs/mediaTypes/expr fields.
type FileEventListenerOptions struct {
	Dirs        []string
	MediaTypes  []sdkconst.MediaType
	Actions     []int
	ExcludeExts []string
	QoS         sdkconst.TaskLevel
	TTL         time.Duration
	Description string
}

// FileEventListener is the handle returned by StartFileEventListener.
type FileEventListener struct {
	monitor *FileEventMonitor
	id      string
	cancel  context.CancelFunc
}

// ListenerID returns the listenerId issued by the OS.
func (e *FileEventListener) ListenerID() string {
	if e == nil {
		return ""
	}
	return e.id
}

// Close stops local consumption and asks the OS to delete the listener.
func (e *FileEventListener) Close() {
	if e == nil || e.monitor == nil {
		return
	}
	if e.cancel != nil {
		e.cancel()
	}
	if l := e.monitor.listeners[e.id]; l != nil && l.controlSub != nil {
		_ = l.controlSub.Unsubscribe()
	}
	_ = e.monitor.deleteListener(e.id)
	e.monitor.mu.Lock()
	delete(e.monitor.listeners, e.id)
	e.monitor.mu.Unlock()
}

type interestListenerSpec struct {
	dirs        []string
	mediaTypes  []string
	actions     []string
	excludeExts []string
	qos         string
	ttl         time.Duration
}

func NewFileEventMonitor(appCode *string, nc *nats.Conn, preferOps ...sdkdto.PreferOptions) *FileEventMonitor {
	fem := &FileEventMonitor{
		appCode:       appCode,
		nc:            nc,
		preferOptions: make(map[string]sdkdto.PreferOptions),
		listeners:     make(map[string]*interestListener),
	}
	for i := range preferOps {
		if preferOps[i].Priority == 0 {
			preferOps[i].Priority = sdkconst.LowLevel
		}
		fem.preferOptions[preferOps[i].ParseToEventCode(*fem.appCode)] = preferOps[i]
	}
	if nc != nil {
		if js, err := jetstream.New(nc); err == nil {
			fem.js = js
		}
	}
	fem.ctx, fem.cancel = context.WithCancel(context.Background())
	if err := fem.start(); err != nil {
		if utils.RuntimeManaged() {
			sdklog.Logger.Warnf("start interest listeners failed: %v; under minictl runtime, file events unavailable", err)
		} else {
			panic(err)
		}
	}
	return fem
}

func (m *FileEventMonitor) Name() string { return "FileEventMonitor" }

func (m *FileEventMonitor) IsReady() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.listeners) > 0
}

func (m *FileEventMonitor) Close() {
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	listeners := make([]*interestListener, 0, len(m.listeners))
	for _, l := range m.listeners {
		listeners = append(listeners, l)
	}
	m.listeners = make(map[string]*interestListener)
	m.mu.Unlock()
	for _, l := range listeners {
		if l.cancel != nil {
			l.cancel()
		}
		_ = m.deleteListener(l.id)
	}
	m.wg.Wait()
}

// FileExistsByEvent checks if the file exists by event
func (m *FileEventMonitor) FileExistsByEvent(ugn *utils.UserGroupNamespace, ufi string) bool {
	api := utils.GetOSServiceURL() + "/app/event/file/exist"
	resp := restclient.GetRequest[bool](ugn, api,
		map[string]string{
			"ufi":     ufi,
			"appCode": *m.appCode,
		})
	if resp.Code != sdkconst.Success {
		return false
	}
	return *resp.Data
}

// GetUserAppEventConfig gets user app event config (legacy config endpoint, still used by apps).
func (m *FileEventMonitor) GetUserAppEventConfig(ugn *utils.UserGroupNamespace) *sdkdto.UserAppEventConfig {
	api := utils.GetOSServiceURL() + "/app/event/config"
	resp := restclient.GetRequest[sdkdto.UserAppEventConfig](ugn, api,
		map[string]string{
			"appCode": *m.appCode,
		})
	if resp.Code != sdkconst.Success {
		return nil
	}
	return resp.Data
}

// SetUserAppEventConfig sets user app event config (legacy config endpoint).
func (m *FileEventMonitor) SetUserAppEventConfig(ugn *utils.UserGroupNamespace, followUFIs []string) error {
	api := utils.GetOSServiceURL() + "/app/event/config"
	params := sdkdto.UserAppEventConfig{
		AppCode:    *m.appCode,
		FollowDirs: followUFIs,
	}
	resp := restclient.PostRequest[any](ugn, api, nil, params)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

// TriggerAppEvents fires the given app eventType for every file under the
// directory identified by ufi.
func (m *FileEventMonitor) TriggerAppEvents(ugn *utils.UserGroupNamespace, ufi string, eventType int, payload any) error {
	raw, err := encodePayload(payload)
	if err != nil {
		return err
	}
	return m.doTriggerAppEvents(ugn, ufi, eventType, raw)
}

func (m *FileEventMonitor) doTriggerAppEvents(ugn *utils.UserGroupNamespace, ufi string, eventType int, payload json.RawMessage) error {
	api := utils.GetOSServiceURL() + "/storage/restore/meta"
	body := map[string]any{
		"ufi":       ufi,
		"onlyEvent": true,
		"appCode":   *m.appCode,
		"eventType": eventType,
	}
	if len(payload) > 0 {
		body["payload"] = payload
	}
	resp := restclient.PostRequest[any](ugn, api, nil, body)
	if resp.Code != sdkconst.Success {
		return errors.New(resp.Msg)
	}
	return nil
}

func encodePayload(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	switch v := payload.(type) {
	case json.RawMessage:
		if len(v) == 0 {
			return nil, nil
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("TriggerAppEvents: payload is json.RawMessage but not valid JSON")
		}
		return v, nil
	case []byte:
		if len(v) == 0 {
			return nil, nil
		}
		if !json.Valid(v) {
			return nil, fmt.Errorf("TriggerAppEvents: payload is []byte but not valid JSON")
		}
		return json.RawMessage(v), nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("TriggerAppEvents: marshal payload: %w", err)
	}
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	return json.RawMessage(data), nil
}

type interestListenResp struct {
	ListenerID     string `json:"listenerId"`
	ChannelID      string `json:"channelId"`
	AppCode        string `json:"appCode"`
	Group          string `json:"group"`
	QoS            string `json:"qos"`
	MaxAck         int    `json:"maxAckPending"`
	Workers        int    `json:"workers"`
	ControlSubject string `json:"controlSubject"`
	JetStream      struct {
		Stream   string `json:"stream"`
		Subject  string `json:"subject"`
		Consumer string `json:"consumer"`
	} `json:"jetstream"`
}

type interestEvent struct {
	V       int                       `json:"v"`
	EventID string                    `json:"eventId"`
	UFI     string                    `json:"ufi"`
	SrcUFI  string                    `json:"srcUfi"`
	Action  int                       `json:"action"`
	Inode   uint64                    `json:"inode"`
	UGN     *utils.UserGroupNamespace `json:"ugn"`
	Matched struct {
		Apps      []string `json:"apps"`
		Listeners []string `json:"listeners"`
	} `json:"matched"`
}

func (m *FileEventMonitor) start() error {
	if m.nc == nil || m.js == nil {
		return errors.New("file event monitor: NATS connection is nil")
	}
	// Legacy delete/rename options may omit FollowDirs. Do not merge all add dirs;
	// that would make delete listeners cover unrelated dirs. Only inherit dirs from add with the same mediaType.
	for _, op := range m.preferOptions {
		dirs := op.FollowDirs
		if len(dirs) == 0 {
			dirs = m.addDirsFor(op.MediaType)
		}
		if err := m.startListener(op, dirs); err != nil {
			return err
		}
	}
	return nil
}

// addDirsFor returns the add-listener dirs for the same mediaType, or nil so startListener
// fails. Never fall back to all add dirs or the root dir.
func (m *FileEventMonitor) addDirsFor(mt sdkconst.MediaType) []string {
	for _, op := range m.preferOptions {
		if op.FileEventType != sdkconst.FileEventAdd || op.MediaType != mt {
			continue
		}
		if len(op.FollowDirs) > 0 {
			return op.FollowDirs
		}
	}
	return nil
}

func (m *FileEventMonitor) startListener(op sdkdto.PreferOptions, dirs []string) error {
	actions := make([]string, 0, 1)
	switch op.FileEventType {
	case sdkconst.FileEventAdd:
		actions = append(actions, "add")
	case sdkconst.FileEventDelete:
		actions = append(actions, "delete")
	case sdkconst.FileEventRename:
		actions = append(actions, "rename")
	}
	spec := interestListenerSpec{
		dirs:        dirs,
		mediaTypes:  mediaTypesFor(op.MediaType),
		actions:     actions,
		excludeExts: op.ExcludeExts,
		qos:         qosForPriority(op.Priority),
		ttl:         30 * time.Minute,
	}
	_, err := m.startInterestListener(spec, op.Handler)
	return err
}

// StartFileEventListener is the public listener-oriented entry point: one listener has one set of
// dirs/media/actions/exclude filters, converted into an OS interest expression.
func (m *FileEventMonitor) StartFileEventListener(opts FileEventListenerOptions, handler func(fe *sdkdto.FileEvent) (sdkconst.ConsumeStatus, string)) (*FileEventListener, error) {
	dirs := make([]string, 0, len(opts.Dirs))
	for _, d := range opts.Dirs {
		d = strings.TrimSpace(d)
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	if len(dirs) == 0 {
		return nil, errors.New("StartFileEventListener: dirs must not be empty (root dir is forbidden)")
	}
	if handler == nil {
		return nil, errors.New("StartFileEventListener: handler must not be nil")
	}
	actions := make([]string, 0, len(opts.Actions))
	for _, a := range opts.Actions {
		switch a {
		case sdkconst.FileEventAdd:
			actions = append(actions, "add")
		case sdkconst.FileEventDelete:
			actions = append(actions, "delete")
		case sdkconst.FileEventRename:
			actions = append(actions, "rename")
		}
	}
	mediaTypes := make([]string, 0, len(opts.MediaTypes))
	for _, mt := range opts.MediaTypes {
		if s := strings.TrimSpace(string(mt)); s != "" {
			mediaTypes = append(mediaTypes, s)
		}
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	spec := interestListenerSpec{
		dirs: dirs, mediaTypes: mediaTypes, actions: actions,
		excludeExts: opts.ExcludeExts, qos: qosForPriority(opts.QoS), ttl: ttl,
	}
	return m.startInterestListener(spec, handler)
}

func (m *FileEventMonitor) startInterestListener(spec interestListenerSpec, handler func(fe *sdkdto.FileEvent) (sdkconst.ConsumeStatus, string)) (*FileEventListener, error) {
	if len(spec.dirs) == 0 {
		return nil, errors.New("file event listener: dirs must not be empty (root dir is forbidden)")
	}
	for _, d := range spec.dirs {
		if strings.TrimSpace(d) == "*" || strings.TrimSpace(d) == "/" {
			return nil, fmt.Errorf("file event listener: target mode does not allow root/wildcard dir %q", d)
		}
	}
	expr := buildExprSpec(spec.actions, spec.excludeExts)
	body := map[string]any{
		"appCode":    *m.appCode,
		"group":      *m.appCode,
		"qos":        spec.qos,
		"ttlSec":     int(spec.ttl.Seconds()),
		"dirs":       spec.dirs,
		"mediaTypes": spec.mediaTypes,
	}
	if expr != "" {
		body["expr"] = expr
	}
	api := utils.GetOSServiceURL() + "/interest/listen"
	resp := restclient.PostRequest[interestListenResp](nil, api, nil, body)
	if resp.Code != sdkconst.Success || resp.Data == nil {
		return nil, fmt.Errorf("interest listen failed: code=%s msg=%s", resp.Code, resp.Msg)
	}
	lr := resp.Data
	if lr.ListenerID == "" {
		lr.ListenerID = lr.ChannelID
	}
	initialWorkers := lr.Workers
	if initialWorkers <= 0 {
		initialWorkers = 1
	}
	l := &interestListener{
		id: lr.ListenerID, appCode: *m.appCode,
		consumer: lr.JetStream.Consumer, stream: lr.JetStream.Stream,
		ttl: spec.ttl, handler: handler,
	}
	l.workerLimit.Store(int64(normalizeWorkers(initialWorkers)))
	l.ctx, l.cancel = context.WithCancel(m.ctx)
	stream, err := m.js.Stream(l.ctx, l.stream)
	if err != nil {
		_ = m.deleteListener(l.id)
		return nil, fmt.Errorf("interest listener stream %s: %w", l.stream, err)
	}
	cons, err := stream.Consumer(l.ctx, l.consumer)
	if err != nil {
		_ = m.deleteListener(l.id)
		return nil, fmt.Errorf("interest listener consumer %s: %w", l.consumer, err)
	}
	l.cons = cons
	if m.nc != nil {
		subject := lr.ControlSubject
		if subject == "" {
			subject = "os.interest.control." + l.id
		}
		l.controlSub, _ = m.nc.Subscribe(subject, func(msg *nats.Msg) {
			var ctl struct {
				Workers int `json:"workers"`
			}
			if json.Unmarshal(msg.Data, &ctl) == nil && ctl.Workers > 0 {
				l.workerLimit.Store(int64(normalizeWorkers(ctl.Workers)))
				sdklog.Logger.Infof("[interest] worker limit updated listener=%s workers=%d", l.id, ctl.Workers)
			}
		})
	}
	m.mu.Lock()
	m.listeners[l.id] = l
	m.mu.Unlock()
	m.wg.Add(2)
	go func() { defer m.wg.Done(); m.consumeLoop(l) }()
	go func() { defer m.wg.Done(); m.renewLoop(l) }()
	return &FileEventListener{monitor: m, id: l.id, cancel: l.cancel}, nil
}

func buildExprSpec(actions, excludeExts []string) string {
	parts := make([]string, 0, 2)
	switch len(actions) {
	case 0:
	case 1:
		parts = append(parts, fmt.Sprintf("action == %q", actions[0]))
	default:
		items := make([]string, 0, len(actions))
		for _, a := range actions {
			items = append(items, fmt.Sprintf("%q", a))
		}
		parts = append(parts, fmt.Sprintf("action in [%s]", strings.Join(items, ", ")))
	}
	if len(excludeExts) > 0 {
		items := make([]string, 0, len(excludeExts))
		for _, e := range excludeExts {
			e = strings.ToLower(strings.TrimSpace(e))
			if e == "" {
				continue
			}
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			items = append(items, fmt.Sprintf("%q", e))
		}
		if len(items) > 0 {
			parts = append(parts, fmt.Sprintf("!(ext in [%s])", strings.Join(items, ", ")))
		}
	}
	return strings.Join(parts, " && ")
}

func buildExpr(op sdkdto.PreferOptions) string {
	parts := make([]string, 0, 2)
	switch op.FileEventType {
	case sdkconst.FileEventAdd:
		parts = append(parts, "action == 'add'")
	case sdkconst.FileEventDelete:
		parts = append(parts, "action == 'delete'")
	case sdkconst.FileEventRename:
		parts = append(parts, "action == 'rename'")
	}
	if len(op.ExcludeExts) > 0 {
		items := make([]string, 0, len(op.ExcludeExts))
		for _, e := range op.ExcludeExts {
			e = strings.ToLower(strings.TrimSpace(e))
			if e == "" {
				continue
			}
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			items = append(items, fmt.Sprintf("%q", e))
		}
		if len(items) > 0 {
			parts = append(parts, fmt.Sprintf("!(ext in [%s])", strings.Join(items, ", ")))
		}
	}
	return strings.Join(parts, " && ")
}

func mediaTypesFor(mt sdkconst.MediaType) []string {
	if strings.TrimSpace(string(mt)) == "" {
		return nil
	}
	return []string{string(mt)}
}

func qosForPriority(p sdkconst.TaskLevel) string {
	switch p {
	case sdkconst.HighLevel:
		return "high"
	case sdkconst.MidLevel:
		return "medium"
	default:
		return "low"
	}
}

func (m *FileEventMonitor) consumeLoop(l *interestListener) {
	for {
		if l.ctx.Err() != nil {
			return
		}
		batch, err := l.cons.Fetch(16, jetstream.FetchMaxWait(500*time.Millisecond))
		if err != nil {
			if l.ctx.Err() != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		workers := int(l.workerLimit.Load())
		if workers < 1 {
			workers = 1
		}
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for msg := range batch.Messages() {
			wg.Add(1)
			sem <- struct{}{}
			go func(msg jetstream.Msg) {
				defer wg.Done()
				defer func() { <-sem }()
				m.handle(l, msg)
			}(msg)
		}
		wg.Wait()
	}
}

func (m *FileEventMonitor) handle(l *interestListener, msg jetstream.Msg) {
	var ev interestEvent
	if err := json.Unmarshal(msg.Data(), &ev); err != nil {
		_ = msg.Ack()
		return
	}
	if !contains(ev.Matched.Listeners, l.id) {
		_ = msg.Ack()
		return
	}
	fe := &sdkdto.FileEvent{
		UGN: ev.UGN, UFI: ev.UFI, SrcUFI: ev.SrcUFI, Inode: ev.Inode,
		Action: ev.Action,
	}
	var cs sdkconst.ConsumeStatus
	var text string
	func() {
		defer func() {
			if r := recover(); r != nil {
				sdklog.Logger.Errorf("FileEventMonitor handler panic: %v\n%s", r, debug.Stack())
				cs = sdkconst.ConsumeFail
			}
		}()
		cs, text = l.handler(fe)
	}()
	m.reportInterestResult(l, ev, cs, text)
	switch cs {
	case sdkconst.ConsumeSuccess:
		_ = msg.Ack()
	case sdkconst.ConsumeRetry:
		_ = msg.Nak()
	default:
		// Business failure is ACKed as terminal to avoid infinite poison-message redelivery; keep the reason in logs.
		sdklog.Logger.Warnf("interest handler failed: app=%s listener=%s ufi=%s msg=%s", l.appCode, l.id, fe.UFI, text)
		_ = msg.Ack()
	}
}

const (
	// interestResultSubject is the fixed subject on which the OS settles interest consumption results.
	interestResultSubject = "os.interest.result"
	// interestResultPublishTimeout bounds JetStream result publishing: reporting happens in the consume callback,
	// so a slow publish must not stall the event stream. Local NATS JS ack is sub-ms; 500ms is only a guard;
	// during the fallback window (old OS without a result stream) each event waits at most this long.
	interestResultPublishTimeout = 500 * time.Millisecond
)

func (m *FileEventMonitor) reportInterestResult(l *interestListener, ev interestEvent, cs sdkconst.ConsumeStatus, text string) {
	if m == nil || m.nc == nil || ev.EventID == "" {
		return
	}
	status := "success"
	switch cs {
	case sdkconst.ConsumeRetry:
		status = "retry"
	case sdkconst.ConsumeFail:
		status = "failed"
	}
	body, _ := json.Marshal(map[string]any{
		"eventId": ev.EventID, "appCode": l.appCode, "listenerId": l.id,
		"status": status, "message": text, "ugn": ev.UGN,
	})
	m.publishInterestResult(body)
}

// publishInterestResult prefers JetStream durable publish: the OS result stream persists, so process
// restarts or disconnects can recover. When JetStream is unavailable (no result stream, publish
// timeout, non-JetStream deployment), fall back to core NATS for old-OS compatibility. Fields and subject are unchanged.
func (m *FileEventMonitor) publishInterestResult(body []byte) {
	if m.nc == nil {
		return
	}
	if m.js != nil {
		ctx, cancel := context.WithTimeout(context.Background(), interestResultPublishTimeout)
		_, err := m.js.Publish(ctx, interestResultSubject, body)
		cancel()
		if err == nil {
			return
		}
		sdklog.Logger.Debugf("interest result: JetStream publish failed, falling back to core NATS: %v", err)
	}
	_ = m.nc.Publish(interestResultSubject, body)
}

func (m *FileEventMonitor) renewLoop(l *interestListener) {
	ticker := time.NewTicker(l.ttl / 3)
	defer ticker.Stop()
	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			api := utils.GetOSServiceURL() + "/interest/listen/" + l.id + "/renew"
			resp := restclient.PostRequest[any](nil, api, nil, map[string]any{"ttlSec": int(l.ttl.Seconds())})
			if resp.Code != sdkconst.Success {
				sdklog.Logger.Warnf("interest listener renew failed: id=%s code=%s msg=%s", l.id, resp.Code, resp.Msg)
			}
		}
	}
}

func (m *FileEventMonitor) deleteListener(id string) error {
	api := utils.GetOSServiceURL() + "/interest/listen/" + id
	if restclient.Client == nil {
		return nil
	}
	resp, err := restclient.Client.R().Delete(api)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("delete interest listener %s: status=%d body=%s", id, resp.StatusCode(), resp.String())
	}
	return nil
}

func contains(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}

func normalizeWorkers(n int) int {
	if n <= 1 {
		return 1
	}
	if n > 64 {
		return 64
	}
	return n
}

func actionCode(action string) int {
	switch strings.ToLower(action) {
	case "add":
		return sdkconst.FileEventAdd
	case "delete":
		return sdkconst.FileEventDelete
	case "rename":
		return sdkconst.FileEventRename
	default:
		return 0
	}
}
