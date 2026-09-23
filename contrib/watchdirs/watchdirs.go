// Package watchdirs exposes the SDK's file event watch scope (app.Events()) as an
// application-facing user interface, `GET/POST <prefix>/dirs`, so an application does not
// have to write the "read, write, validate, map error codes" round itself.
//
// Why it is needed: each of the four FaaS applications maintained its own `/watch/dirs` —
// reading the application-scoped KV, normalizing user input into a UFI, and then filtering
// event paths by it. After v4 the scope is a platform capability (cap), persisted by
// AppStore and executed by the daemon; one more copy per application would mean three
// implementations. This package takes it over:
//
//	watchdirs.New(app, watchdirs.Options{Scope: spaceOf}).Register(server, "/v2")
//
// What the user still sees is a "directory" (`/Music/2024`), while storage and filtering use
// scope values (`space.music/2024`). The conversion goes through the protocol layer's key
// table (`/Music` ⇄ `music`), so "the directory shown in the UI" and "the scope actually in
// effect" never disagree.
package watchdirs

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/kataras/iris/v12"

	rj "github.com/siyouyun-open/siyouyun_sdk/contrib/restjson"
	"github.com/siyouyun-open/siyouyun_sdk/v2"
	"github.com/siyouyun-open/siyouyun_sdk/v2/capability"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// The user-visible spelling of the whole space: a `/` in the dir list means "no restriction
// within the authorized space".
const wholeSpaceDir = "/"

// Options configures the handler.
type Options struct {
	// Scope resolves the user space scope of this request (required for native; a container
	// may leave it empty and let the daemon derive it).
	//
	// Returning false means "a response has already been written" (for example, the
	// application's own identity check failed), and the handler returns immediately.
	Scope func(iris.Context) (capability.Scope, bool)
	// Defaults are the dirs shown to the user when cap is unavailable (old devices); they
	// are also the fallback when nothing has ever been configured.
	Defaults []string
	// Logger falls back to the standard log when nil.
	Logger *log.Logger
}

// Handler implements `/watch/dirs`.
type Handler struct {
	app    *appsdk.App
	opts   Options
	logger *log.Logger
}

// New creates a handler. When app is nil every request returns 503 (bus unavailable).
func New(app *appsdk.App, opts Options) *Handler {
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Handler{app: app, opts: opts, logger: logger}
}

// Register mounts the routes on party:
//
//	GET  <prefix>/dirs         current watch dirs (array, compatible with the old frontend)
//	POST <prefix>/dirs         change the watch dirs
//	GET  <prefix>/dirs/options dirs + scope values + ceiling (for the "add dir" dropdown)
//
// An empty prefix means mounting at the root.
func (h *Handler) Register(party iris.Party, prefix string) {
	if party == nil {
		return
	}
	base := strings.TrimRight(strings.TrimSpace(prefix), "/")
	party.Get(base+"/dirs", h.Get)
	party.Post(base+"/dirs", h.Set)
	party.Get(base+"/dirs/options", h.Options)
}

// Get returns the current watch dirs; it degrades to the default dirs when cap is
// unavailable.
func (h *Handler) Get(ctx iris.Context) {
	status, ok := h.status(ctx)
	if !ok {
		return
	}
	if status == nil {
		dirs := normalizeDirs(h.opts.Defaults)
		ctx.JSON(rj.SuccessResJsonWithData(&dirs))
		return
	}
	dirs := dirsFromScopes(status.Scopes)
	ctx.JSON(rj.SuccessResJsonWithData(&dirs))
}

// Options returns dirs, scope values and the ceiling together, for the UI to render the
// "dirs that can be added".
func (h *Handler) Options(ctx iris.Context) {
	status, ok := h.status(ctx)
	if !ok {
		return
	}
	payload := map[string]any{
		"dirs":     normalizeDirs(h.opts.Defaults),
		"scopes":   []string{},
		"ceiling":  []string{},
		"degraded": true,
	}
	if status != nil {
		payload["dirs"] = dirsFromScopes(status.Scopes)
		payload["scopes"] = status.Scopes
		payload["ceiling"] = status.Ceiling
		payload["updatedBy"] = status.UpdatedBy
		payload["degraded"] = false
		// The selectable dirs the ceiling allows: the UI uses this to decide whether a given
		// dir can be added.
		payload["ceilingDirs"] = dirsFromScopes(status.Ceiling)
	}
	ctx.JSON(rj.SuccessResJsonWithData(&payload))
}

// Set changes the watch dirs.
//
// Semantics (deliberately different from the old implementation, and they must be explicit):
//   - `{"dirs": null}` / `{"scopes": null}` → back to the manifest ceiling;
//   - `[]` → **the empty set**: watch no dir at all (the old implementation read this as "no
//     restriction"; it is the opposite now, because "no restriction" is expressed as
//     `["/"]`);
//   - `["/"]` → no restriction within the authorized space (= the widest value in the
//     ceiling);
//   - neither field given → 400: better to do nothing than to guess whether the user wants to
//     loosen or tighten.
func (h *Handler) Set(ctx iris.Context) {
	events, capOK, ok := h.events(ctx)
	if !ok {
		return
	}
	if !capOK || events == nil {
		ctx.JSON(rj.ErrorResJsonWithMsg("this device does not support changing watch directories yet"))
		return
	}

	raw, specified, ok := readRequestValues(ctx)
	if !ok {
		ctx.JSON(rj.ErrorResJsonWithMsg("either dirs or scopes is required"))
		return
	}

	var (
		scope []string
		err   error
	)
	if specified && raw == nil {
		// An explicit null = back to the manifest ceiling: **no values passed** (the SDK sends
		// JSON null).
		scope, err = events.SetScopes(ctx.Request().Context())
	} else {
		converted, convErr := toScopeValues(raw)
		if convErr != nil {
			// A user input problem: return the real reason to the UI ("unknown dir" and the
			// like) rather than a generic "operation failed".
			ctx.JSON(rj.ErrorResJsonWithMsg(convErr.Error()))
			return
		}
		scope, err = events.SetScopes(ctx.Request().Context(), converted...)
	}
	if err != nil {
		h.logger.Printf("[watchdirs] failed to set watch directories: %v", err)
		// Out of ceiling is a user-correctable error: return it verbatim to the UI so it can
		// say "this dir is not within the application's scope".
		ctx.JSON(rj.ErrorResJsonWithMsg(err.Error()))
		return
	}
	dirs := dirsFromScopes(scope)
	ctx.JSON(rj.SuccessResJsonWithData(&dirs))
}

// readRequestValues reads the dirs / scope values from the request.
//
// Return values: raw is the raw input; specified means the request **explicitly** provided
// this field (distinguishing null from absent); ok=false means neither field was given and
// the caller should report 400.
//
// RawMessage is used instead of a struct because only that way can "field absent" (the
// frontend missed it → 400) be told apart from "explicit null" (back to the ceiling). The two
// are completely different actions on the backend and must not be conflated.
func readRequestValues(ctx iris.Context) (raw []string, specified bool, ok bool) {
	var body map[string]json.RawMessage
	if err := ctx.ReadJSON(&body); err == nil {
		if value, present := body["scopes"]; present {
			return decodeStringList(value)
		}
		if value, present := body["dirs"]; present {
			return decodeStringList(value)
		}
		return nil, false, false
	}
	// Compatible with the old frontend's repeated query parameters:
	// `?dirs=/Music&dirs=/Photos` or comma-separated.
	var fromQuery []string
	for _, value := range ctx.Request().URL.Query()["dirs"] {
		fromQuery = append(fromQuery, strings.Split(value, ",")...)
	}
	if len(fromQuery) > 0 {
		return fromQuery, true, true
	}
	return nil, false, false
}

// decodeStringList parses `null` / `[]` / `["/Music"]`.
//
// null → (nil, true): back to the ceiling; [] → (empty slice, true): the empty set.
func decodeStringList(raw json.RawMessage) ([]string, bool, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, true, true
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, false, false
	}
	if list == nil {
		list = []string{}
	}
	return list, true, true
}

// toScopeValues converts user input (dirs or scope values) into a list of scope values.
//
// Both kinds of input are accepted:
//   - already a scope value (`space.music/2024`, `space`, `all.photos`);
//   - a dir (`/Music/2024`), including the old frontend's UFI shape
//     (`/ufi/pool-raw/syspool/Music`).
//
// An empty list is returned as is (the caller uses it to express "the empty set").
func toScopeValues(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		value, err := scopeValueFromInput(item)
		if err != nil {
			return nil, err
		}
		canonical := value.String()
		if _, dup := seen[canonical]; dup {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, canonical)
	}
	if len(out) == 0 {
		return []string{}, nil
	}
	return out, nil
}

// scopeValueFromInput converts a single input into a scope value.
func scopeValueFromInput(item string) (protocol.ScopeValue, error) {
	// 1) The dir spelling of the whole space.
	if item == wholeSpaceDir || item == "/ufi" {
		return protocol.ScopeValue{Space: protocol.ScopeSpaceOwn}, nil
	}
	// 2) Is it already a scope value?
	if looksLikeScopeValue(item) {
		return protocol.ParseScopeValue(item)
	}
	// 3) A dir (including a UFI passed by the old frontend).
	dir := item
	if rel, ok := protocol.UFIPoolRel(item); ok {
		dir = rel
	}
	key, ok := protocol.ScopeKeyForDir(dir)
	if !ok {
		return protocol.ScopeValue{}, &unknownDirError{input: item}
	}
	return protocol.ParseScopeValue(string(protocol.ScopeSpaceOwn) + "." + key)
}

// looksLikeScopeValue reports whether the input is written as a scope value.
//
// The criteria: `space`/`app`/`all`, or `<space>.<key>`. A dir always starts with `/`, so
// there is no misclassification.
func looksLikeScopeValue(item string) bool {
	lower := strings.ToLower(item)
	switch lower {
	case string(protocol.ScopeSpaceOwn), string(protocol.ScopeSpaceApp), string(protocol.ScopeSpaceAll):
		return true
	}
	head := lower
	if idx := strings.IndexByte(lower, '.'); idx >= 0 {
		head = lower[:idx]
	} else {
		return false
	}
	switch protocol.ScopeSpace(head) {
	case protocol.ScopeSpaceOwn, protocol.ScopeSpaceApp, protocol.ScopeSpaceAll:
		return true
	}
	return false
}

// unknownDirError means "this dir is not in the platform's registered dir table".
type unknownDirError struct{ input string }

func (e *unknownDirError) Error() string {
	return "unsupported directory " + e.input + " (available: " + strings.Join(availableDirs(), ", ") + ")"
}

// availableDirs lists the platform's registered dirs (used in error messages).
func availableDirs() []string {
	keys := protocol.ScopeKeyNames()
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if dir, ok := protocol.ScopeKeyDir(key); ok {
			out = append(out, dir)
		}
	}
	return out
}

// DirsFromScopes converts scope values into user-visible dirs (stable order, the whole space
// is `/`).
//
// For applications whose configuration interface still speaks in dirs (for example, an album
// reading the watched dirs out of its own settings panel), so that the UI vocabulary and the
// effective scope always come from the same conversion.
func DirsFromScopes(values []string) []string { return dirsFromScopes(values) }

// ScopeValuesFromDirs converts user-supplied dirs (or scope values) into canonical scope
// values.
//
// Empty input returns an empty list (= the empty set); to express "back to the ceiling" the
// caller should pass nil explicitly.
func ScopeValuesFromDirs(dirs []string) ([]string, error) { return toScopeValues(dirs) }

// dirsFromScopes converts scope values into user-visible dirs (stable order, the whole space
// is `/`).
func dirsFromScopes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value, err := protocol.ParseScopeValue(raw)
		if err != nil {
			continue
		}
		if value.Space == protocol.ScopeSpaceApp {
			// Application-private dirs are not "user-selectable dirs".
			continue
		}
		if path, ok := value.ScopeKeyPath(); ok {
			out = append(out, path)
		}
	}
	return normalizeDirs(out)
}

func normalizeDirs(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	seen := make(map[string]struct{}, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if dir != wholeSpaceDir {
			dir = "/" + strings.Trim(dir, "/")
		}
		if _, dup := seen[dir]; dup {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}

// events resolves the event view for this request.
//
// The second return value reports whether "cap is available": when it is false (nil events)
// the caller degrades to read-only default dirs.
func (h *Handler) events(ctx iris.Context) (*capability.Events, bool, bool) {
	if h.app == nil {
		ctx.StatusCode(http.StatusServiceUnavailable)
		ctx.JSON(rj.ErrorResJsonWithMsg("the bus is unavailable"))
		return nil, false, false
	}
	scope := capability.Scope{}
	if h.opts.Scope != nil {
		resolved, ok := h.opts.Scope(ctx)
		if !ok {
			return nil, false, false // a response has already been written
		}
		scope = resolved
	}
	return h.app.Events().For(scope), true, true
}

// status reads the scope; when cap is unavailable it returns (nil, true) so the caller takes
// the degraded branch.
func (h *Handler) status(ctx iris.Context) (*capability.ScopeStatus, bool) {
	events, capOK, ok := h.events(ctx)
	if !ok {
		return nil, false
	}
	if !capOK || events == nil {
		return nil, true
	}
	status, err := events.Status(ctx.Request().Context())
	if err != nil {
		h.logger.Printf("[watchdirs] failed to read the watch scope (falling back to the default directories): %v", err)
		return nil, true
	}
	return &status, true
}
