package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	appfs "github.com/siyouyun-open/siyouyun_sdk/v2/fs"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// User-facing interface for file events (design draft v4 §5, docs/specs/file-event-permission-model.md).
//
// The app side only needs three things:
//
//	sub, err := app.Events().For(scope).Subscribe(ctx, handler)   // subscribe
//	ceiling, err := app.Events().For(scope).Ceiling(ctx)          // ceiling (manifest)
//	err = app.Events().For(scope).SetScopes(ctx, "space.music/2024") // change watched directories
//
// The watched scope has sources of truth on both ends: AppStore persistence (L2) and daemon-side
// execution filtering. What the SDK does here is **local defensive filtering** — when the daemon
// has not yet received the new configuration (or is temporarily unreachable), it still does not
// hand events from the old scope to business code; at the same time it centralizes the
// "value → path prefix" check, so apps no longer each maintain their own NormalizeDir/InDirs.

// ErrScopeOutOfCeiling reports that the requested watched scope exceeds the ceiling declared in
// the manifest.
//
// This is an error the **caller can fix**: the UI should say "this directory is not within the
// app's watched scope" rather than treating it as a system failure. The daemon returns 400 for an
// out-of-ceiling request, and the SDK maps it to this sentinel error.
var ErrScopeOutOfCeiling = errors.New("event scope out of manifest ceiling")

// ErrEventScopeUnsupported reports that the AppStore/daemon on the device does not yet support the
// watched-scope capability (an older version). The caller should degrade to "local filtering only,
// not configurable" rather than treating it as a fatal error.
var ErrEventScopeUnsupported = errors.New("event scope capability is not available on this device")

// scopeCacheTTL is how long "the effective scope of a space" is cached in-process.
//
// Every event path has to be checked against directories, and it cannot ask the bus for each one;
// but a user's scope change should also take effect quickly, hence the short TTL. `SetScopes`
// refreshes the cache for the corresponding space immediately, unaffected by the TTL.
const scopeCacheTTL = 30 * time.Second

// spaceScopes is the scope cache for one deployment space.
type spaceScopes struct {
	values  []protocol.ScopeValue
	fetched bool
	at      time.Time
}

// Events is the entry point for the file event capability.
//
// The cache is split **per space**: native is a shared instance that is "installed once and serves
// multiple spaces", and a single process has to serve the watched scopes of several spaces at once;
// caching per space both keeps event-path checks fast and means that when a user changes
// directories in one space only that space is refreshed.
type Events struct {
	client *bus.Client
	// scope is used only by the For() view (the user-facing path: it tells the platform "which
	// space's" scope is being read or changed).
	scope Scope
	// spaces returns this instance's authorized spaces (for native they come from the mount table;
	// a container has exactly one). It is used to decide whether the space axis of a `space.*` scope
	// matches: you **must not** use "the event's own space" as the baseline, because that would
	// always match (an event of course belongs to its own space).
	spaces func() []string

	// Shared cache: multiple For() views in the same process share one per-space scope cache.
	shared *eventsShared
}

// eventsShared is the mutable state shared between For() views.
type eventsShared struct {
	mu      sync.Mutex
	bySpace map[string]*spaceScopes
}

func (s *eventsShared) cache(space string) *spaceScopes {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bySpace == nil {
		s.bySpace = make(map[string]*spaceScopes)
	}
	entry := s.bySpace[space]
	if entry == nil {
		entry = &spaceScopes{}
		s.bySpace[space] = entry
	}
	return entry
}

// NewEvents creates the file event entry point.
func NewEvents(client *bus.Client) *Events {
	return &Events{client: client, shared: &eventsShared{}}
}

// WithSpaces injects the source of "this instance's authorized spaces" (for native it is provided
// by the mount table).
//
// Passing nil means unknown: the space axis is then not checked (events pass through), following
// the same principle as "if the scope cannot be read, let events through".
func (e *Events) WithSpaces(fn func() []string) *Events {
	if e != nil {
		e.spaces = fn
	}
	return e
}

// For binds a user scope and returns the event view for that space.
//
// native is a shared instance that is "installed once and serves multiple spaces", so the
// user-facing API must be called per space; container apps may leave it empty (the daemon derives
// it from the connection), but passing it explicitly is clearer.
func (e *Events) For(scope Scope) *Events {
	if e == nil {
		return &Events{scope: scope, shared: &eventsShared{}}
	}
	// Share the same per-space cache: repeatedly calling For for the same space should not ask the
	// bus every time.
	return &Events{client: e.client, scope: scope, spaces: e.spaces, shared: e.shared}
}

// ScopeStatus is the result of one scope query.
type ScopeStatus struct {
	// AppCode is the target app resolved by the platform (the install directory name).
	AppCode string `json:"appCode"`
	// Namespace is the deployment space this query targets (`{owner}-{namespace}`).
	Namespace string `json:"namespace"`
	// Ceiling is the manifest ceiling (L1).
	Ceiling []string `json:"ceiling"`
	// Scopes is the effective set (L2); when never configured it equals the watchable part of the
	// ceiling.
	Scopes []string `json:"scope"`
	// UpdatedBy records the source of the most recent change (manifest|user|admin).
	UpdatedBy string `json:"updatedBy"`
	// Source is the daemon-side observation: appstore=delivered; manifest-fallback-*=not yet
	// delivered.
	Source string `json:"source"`
}

type eventScopeCallResponse struct {
	Success   bool     `json:"success"`
	Error     string   `json:"error"`
	AppCode   string   `json:"appCode"`
	Namespace string   `json:"namespace"`
	Ceiling   []string `json:"ceiling"`
	Scope     []string `json:"scope"`
	UpdatedBy string   `json:"updatedBy"`
	Source    string   `json:"source"`
}

// Status reads the ceiling and the effective set (everything in one call).
func (e *Events) Status(ctx context.Context) (ScopeStatus, error) {
	payload := e.scope.payload()
	var res eventScopeCallResponse
	if err := e.callScope(ctx, protocol.TemplateEventScopeGet, payload, &res); err != nil {
		return ScopeStatus{}, err
	}
	if !res.Success && strings.TrimSpace(res.Error) != "" {
		return ScopeStatus{}, errors.New(res.Error)
	}
	status := ScopeStatus{
		AppCode:   res.AppCode,
		Namespace: res.Namespace,
		Ceiling:   append([]string(nil), res.Ceiling...),
		Scopes:    append([]string(nil), res.Scope...),
		UpdatedBy: res.UpdatedBy,
		Source:    res.Source,
	}
	e.cacheScopes(e.deploySpace(), status.Scopes)
	return status, nil
}

// Ceiling reads only the ceiling (L1).
func (e *Events) Ceiling(ctx context.Context) ([]string, error) {
	status, err := e.Status(ctx)
	if err != nil {
		return nil, err
	}
	return status.Ceiling, nil
}

// Scopes reads only the effective set (L2) and refreshes the local filter cache.
func (e *Events) Scopes(ctx context.Context) ([]string, error) {
	status, err := e.Status(ctx)
	if err != nil {
		return nil, err
	}
	return status.Scopes, nil
}

// SetScopes overwrites the watched scope.
//
// Passing no values = **reset to the manifest ceiling** (`values: null`); explicitly passing an
// empty slice (`SetScopes(ctx, []string{}...)`) = the **empty set** (watch no directories). The two
// are different states in the daemon, in what AppStore persists, and in local filtering, and must
// not be conflated.
func (e *Events) SetScopes(ctx context.Context, values ...string) ([]string, error) {
	return e.setScopes(ctx, "set", values)
}

// AddScopes appends to the current effective set.
func (e *Events) AddScopes(ctx context.Context, values ...string) ([]string, error) {
	return e.setScopes(ctx, "add", values)
}

// RemoveScopes removes values from the effective set; removing down to empty means "watch no
// directories".
func (e *Events) RemoveScopes(ctx context.Context, values ...string) ([]string, error) {
	return e.setScopes(ctx, "remove", values)
}

func (e *Events) setScopes(ctx context.Context, op string, values []string) ([]string, error) {
	payload := e.scope.payload()
	payload["op"] = op
	if values == nil {
		// No values = JSON null = reset to the manifest ceiling.
		payload["values"] = nil
	} else {
		// Explicit array (**keep empty arrays**): an empty array = the empty set = watch no
		// directories. Do not use append([]string(nil), values...) here — an empty slice would be
		// collapsed to nil, and "the user deleted all directories" would be silently restored to
		// the default scope.
		out := make([]string, len(values))
		copy(out, values)
		payload["values"] = out
	}

	var res eventScopeCallResponse
	err := e.callScope(ctx, protocol.TemplateEventScopeSet, payload, &res)
	// Delivery failed but the change was persisted: the daemon degrades to not filtering and local
	// filtering takes effect with the new values immediately, so refresh the cache here rather than
	// letting the caller think "nothing happened".
	if err == nil && res.Scope != nil {
		e.cacheScopes(e.deploySpace(), res.Scope)
	}
	return res.Scope, err
}

// Subscribe subscribes to file events and applies local defensive filtering according to the
// effective scope of the **space the event belongs to**.
//
// Loading is lazy and per space: the first event from a space triggers one scope read (reused
// within the TTL), so a native shared instance does not need to subscribe once per space
// (subscribing to the same topic multiple times only splits events among multiple consumers). On
// read failure events pass through — the daemon is already filtering with its own copy, and
// dropping them again locally would only drop more.
func (e *Events) Subscribe(ctx context.Context, handler func(FileEvent)) (*Subscription, error) {
	// A view with an explicit scope (the user-facing path) fetches once up front so local filtering
	// is immediately available.
	if !e.scope.IsZero() {
		_, _ = e.Status(ctx)
	}
	return SubscribeFileEvents(ctx, e.client, func(event FileEvent) {
		if handler == nil {
			return
		}
		if !e.matchesEvent(ctx, event) {
			return
		}
		handler(event)
	})
}

// matchesEvent reports whether the event falls within the effective scope of its own space.
func (e *Events) matchesEvent(ctx context.Context, event FileEvent) bool {
	if e == nil || e.shared == nil {
		return true
	}
	space := event.Space()
	scopes, fetched, stale := e.scopesForSpace(space)
	if !fetched || stale {
		// Lazy load / refresh after expiry: use the UGN carried by the event to ask for this
		// space's scope. On fetch failure keep the old values (if any) — a slightly stale scope is
		// better than silently letting everything through.
		if values, err := e.fetchScopes(ctx, event.Scope()); err == nil {
			scopes, fetched = values, true
		}
	}
	if !fetched {
		return true // never read = let through (defensive filtering, not authoritative)
	}
	return MatchesScopes(e.authorizedSpaces(), scopes, event)
}

// Matches reports whether an event falls within the locally cached effective scope (it looks up
// the cache using the space carried by the event).
//
// It returns true (lets the event through) when the scope has not been read successfully: local
// filtering is defensive, not authoritative. Apps should also use it on the **full reconciliation**
// path, so that the churn of "an event was received but reconciliation cleared it again" does not
// occur.
func (e *Events) Matches(event FileEvent) bool {
	if e == nil || e.shared == nil {
		return true
	}
	scopes, fetched, _ := e.scopesForSpace(event.Space())
	if !fetched {
		return true
	}
	return MatchesScopes(e.authorizedSpaces(), scopes, event)
}

// MatchesScopes is the pure-function form of local filtering: given "this instance's authorized
// space set", one effective scope and one event, it reports whether the event matches. The event
// path and the reconciliation path must use the same logic.
//
// An empty scopes means "watch no directories" (no match); an empty spaces means the authorized
// spaces are unknown, in which case only the path is checked (the same principle as "if the scope
// cannot be read, let events through").
func MatchesScopes(spaces []string, scopes []protocol.ScopeValue, event FileEvent) bool {
	for _, scope := range scopes {
		if matchesScope(spaces, scope, event) {
			return true
		}
	}
	return false
}

// authorizedSpaces returns this instance's authorized spaces; nil when unknown.
func (e *Events) authorizedSpaces() []string {
	if e == nil || e.spaces == nil {
		return nil
	}
	return e.spaces()
}

// scopesForSpace reads the cache; the third return value means "expired, worth fetching again".
func (e *Events) scopesForSpace(space string) ([]protocol.ScopeValue, bool, bool) {
	entry := e.shared.cache(space)
	e.shared.mu.Lock()
	defer e.shared.mu.Unlock()
	if !entry.fetched {
		return nil, false, false
	}
	return entry.values, true, time.Since(entry.at) > scopeCacheTTL
}

// fetchScopes asks the platform directly for a space's scope and writes it to the cache.
func (e *Events) fetchScopes(ctx context.Context, scope Scope) ([]protocol.ScopeValue, error) {
	view := e.For(scope)
	status, err := view.Status(ctx)
	if err != nil {
		return nil, err
	}
	values, err := ScopesFromStrings(status.Scopes)
	if err != nil {
		return nil, err
	}
	return values, nil
}

// ScopesFromStrings parses a list of scope values; any invalid one returns an error.
func ScopesFromStrings(values []string) ([]protocol.ScopeValue, error) {
	out := make([]protocol.ScopeValue, 0, len(values))
	for _, raw := range values {
		value, err := protocol.ParseScopeValue(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

// matchesScope reports whether one scope value covers the event.
func matchesScope(spaces []string, scope protocol.ScopeValue, event FileEvent) bool {
	switch scope.Space {
	case protocol.ScopeSpaceAll:
		// Device level: the space axis always passes; only narrow by path.
	case protocol.ScopeSpaceOwn:
		// Space axis: the event must come from this instance's authorized spaces. When the
		// authorized spaces are unknown (old package / missing mount table) this check is skipped —
		// the daemon only delivers events from authorized spaces.
		if len(spaces) > 0 && !containsString(spaces, event.Space()) {
			return false
		}
	case protocol.ScopeSpaceApp:
		// App-private directories do not produce file events; pass conservatively.
		return true
	default:
		return false
	}
	if scope.WholeSpace() {
		return true
	}
	want, ok := scope.ScopeKeyPath()
	if !ok {
		return false
	}
	for _, ufi := range []string{event.UFI, event.SrcUFI} {
		rel, ok := protocol.UFIPoolRel(ufi)
		if !ok {
			continue
		}
		if protocol.PathWithin(want, rel) {
			return true
		}
	}
	return false
}

// deploySpace returns the deployment space of this view (`{owner}-{namespace}`).
func (e *Events) deploySpace() string {
	if e == nil {
		return ""
	}
	return appfs.DeploySpace(e.scope.Username, e.scope.GroupName, e.scope.Namespace)
}

// cacheScopes writes a space's effective scope to the cache.
//
// It writes only when parsing succeeds: **the empty set is also a valid state** (= watch no
// directories) and must not be treated as "never read" and let through.
func (e *Events) cacheScopes(space string, scopes []string) {
	parsed, err := ScopesFromStrings(scopes)
	if err != nil {
		return
	}
	entry := e.shared.cache(space)
	e.shared.mu.Lock()
	defer e.shared.mu.Unlock()
	entry.values = parsed
	entry.fetched = true
	entry.at = time.Now()
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// callScope invokes the watched-scope capability and decodes the provider's response.
//
// The two error paths must be kept separate:
//   - no provider (old AppStore/daemon) → ErrEventScopeUnsupported, the caller degrades;
//   - out of ceiling (HTTP 400 / the message mentions ceiling) → ErrScopeOutOfCeiling, the caller
//     tells the user.
func (e *Events) callScope(ctx context.Context, template string, payload map[string]any, out *eventScopeCallResponse) error {
	if e == nil || e.client == nil {
		return ErrEventScopeUnsupported
	}
	resp, err := e.client.Call(ctx, &bus.Request{Template: template, Payload: payload})
	if err != nil {
		// When the provider returns non-2xx, the daemon folds it into a single error frame
		// (provider error: 400, body: {...}), so both "out of ceiling" and "no provider" can only
		// be recognized from the message text.
		msg := trimmedError(err)
		if looksLikeCeilingRejection(msg) {
			return fmt.Errorf("%w: %s", ErrScopeOutOfCeiling, msg)
		}
		if isMissingProvider(err) {
			return fmt.Errorf("%w: %s", ErrEventScopeUnsupported, msg)
		}
		return err
	}
	if resp == nil {
		return ErrEventScopeUnsupported
	}
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, out); err != nil {
			return fmt.Errorf("decode event scope response: %w", err)
		}
	}
	if !out.Success {
		msg := strings.TrimSpace(out.Error)
		if msg == "" {
			msg = "event scope call failed"
		}
		if looksLikeCeilingRejection(msg) {
			return fmt.Errorf("%w: %s", ErrScopeOutOfCeiling, msg)
		}
		return errors.New(msg)
	}
	return nil
}

// isMissingProvider recognizes the several phrasings of "this device does not have this capability
// yet".
func isMissingProvider(err error) bool {
	msg := strings.ToLower(trimmedError(err))
	for _, marker := range []string{"no provider found", "no healthy provider", "unknown topic", "unknown template", "not in topic registry"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// looksLikeCeilingRejection decides whether an error is an "over the ceiling" error.
//
// Both the daemon and AppStore phrase these as "not within the manifest's watched scope / over the
// manifest ceiling / missing the corresponding data access scope", and neither includes a
// structured error code (capability calls carry only strings), so it can only be recognized from
// the message text; when it cannot be recognized it degrades to an ordinary error and does not
// present a system failure as a user error.
func looksLikeCeilingRejection(msg string) bool {
	msg = strings.ToLower(strings.TrimSpace(msg))
	for _, marker := range []string{"ceiling", "监听范围", "上限", "越界", "out of"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func trimmedError(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

// SortScopes returns sorted, de-duplicated scope values (for display and comparison).
func SortScopes(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, dup := seen[raw]; dup {
			continue
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	sort.Strings(out)
	return out
}
