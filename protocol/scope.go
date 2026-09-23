// Package protocol is the compile-time contract between the SDK and the daemon: capability
// templates, wire structures, the mount table, and the **syntax of storage/event scope
// values**.
//
// Scope values live here instead of being written once per consumer: the same string
// `space.music/2024` has three interpreters — local filtering in the application (SDK),
// daemon fan-out filtering, and declaration-time validation of the manifest (pkg/types). If
// each of the three implemented the syntax on its own, they would eventually diverge on
// details like "dot or slash" and "is omission equivalent to all", and the symptom of such
// divergence is the extremely hard-to-diagnose problem of "the user changed the watch
// directory but the event behavior is wrong".
//
// Syntax (design doc v4 §3, docs/specs/file-event-permission-model.md):
//
//	scope value := <space> [ "." <path> ]
//	space       := space | app | all
//	path        := <business directory key> [ "/" <relative subpath> ] | all   (omitted = all)
//
// Two hard rules:
//   - the space axis and the path axis each have a clear "all" meaning and are never mixed;
//     a dot only separates levels, and subdirectories always use a slash;
//   - only platform-registered business directory keys are accepted, so there is no
//     arbitrary path escape.
package protocol

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// ScopeSpace is the space axis of a scope value.
type ScopeSpace string

const (
	// ScopeSpaceOwn is the **authorized space set** of the application: for a container =
	// the deployment space; for native = the instance WebNamespaces.
	// Corresponds to the historical token storage.user.file.
	ScopeSpaceOwn ScopeSpace = "space"
	// ScopeSpaceApp is the application's private persistent directory (orthogonal to spaces,
	// granted as a whole, does not accept path narrowing).
	// Corresponds to the historical token storage.app.file.
	ScopeSpaceApp ScopeSpace = "app"
	// ScopeSpaceAll is **all user spaces** on the device (trusted artifact + default only +
	// not applicable to native).
	// Corresponds to the historical token storage.device.file.
	ScopeSpaceAll ScopeSpace = "all"
)

// scopeKeyDirs maps platform-registered business directory keys to fixed relative paths
// inside a pool.
//
// Only standard directories that actually exist on the device and are created in every pool
// are registered (installing an application does not need to know which pools exist; at
// runtime it expands as "every pool in that space + this relative path"). To add a key, add
// a line here.
var scopeKeyDirs = map[string]string{
	"photos":    "/Photos",
	"music":     "/Music",
	"movies":    "/Movies",
	"documents": "/Documents",
	"downloads": "/Downloads",
	"backup":    "/Backup",
	"desktop":   "/Desktop",
}

// ScopeKeyDir returns the in-pool relative path for a business directory key.
func ScopeKeyDir(key string) (string, bool) {
	dir, ok := scopeKeyDirs[strings.ToLower(strings.TrimSpace(key))]
	return dir, ok
}

// ScopeKeyNames returns all registered business directory keys (stable order, used for
// errors and documentation).
func ScopeKeyNames() []string {
	names := make([]string, 0, len(scopeKeyDirs))
	for name := range scopeKeyDirs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ScopeValue is a parsed scope value.
type ScopeValue struct {
	// Space is the space axis.
	Space ScopeSpace
	// Key is the business directory key; empty means **all** paths in that space (the path
	// axis wildcard).
	Key string
	// Sub is the relative subpath under key, e.g. "/2024/raw"; empty means the whole key.
	Sub string
}

// String renders the scope value back in canonical form.
func (v ScopeValue) String() string {
	out := string(v.Space)
	if v.Key != "" {
		out += "." + v.Key + v.Sub
	}
	return out
}

// IsZero reports whether the value is unset.
func (v ScopeValue) IsZero() bool { return strings.TrimSpace(string(v.Space)) == "" }

// WholeSpace reports whether the value covers "all paths in the space" (path axis wildcard).
func (v ScopeValue) WholeSpace() bool { return v.Key == "" }

// UsesDeviceSpace reports whether the value uses the device-level space axis (`all`, which
// requires a trust gate).
func (v ScopeValue) UsesDeviceSpace() bool { return v.Space == ScopeSpaceAll }

// ParseScopeValue parses a scope value and returns a readable error for invalid input
// (applications can surface it to users directly).
func ParseScopeValue(raw string) (ScopeValue, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ScopeValue{}, fmt.Errorf("scope value must not be empty")
	}
	spacePart, pathPart := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		spacePart, pathPart = s[:i], s[i+1:]
	}
	space := ScopeSpace(strings.ToLower(strings.TrimSpace(spacePart)))
	switch space {
	case ScopeSpaceOwn, ScopeSpaceApp, ScopeSpaceAll:
	default:
		return ScopeValue{}, fmt.Errorf("unknown space axis %q: only %s / %s / %s are supported",
			spacePart, ScopeSpaceOwn, ScopeSpaceApp, ScopeSpaceAll)
	}

	path := strings.TrimSpace(pathPart)
	if path == "" || strings.EqualFold(path, "all") {
		return ScopeValue{Space: space}, nil // path omitted or all — equivalent
	}
	if space == ScopeSpaceApp {
		return ScopeValue{}, fmt.Errorf("the app space does not accept a sub-path (%q): the app-private directory is granted as a whole, use \"app\"", raw)
	}

	key, sub := path, ""
	if i := strings.IndexByte(path, '/'); i >= 0 {
		key, sub = path[:i], path[i:]
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if _, ok := scopeKeyDirs[key]; !ok {
		return ScopeValue{}, fmt.Errorf("unknown directory key %q (available: %s)", key, strings.Join(ScopeKeyNames(), ", "))
	}
	if sub != "" {
		clean, err := normalizeScopeSub(sub)
		if err != nil {
			return ScopeValue{}, fmt.Errorf("sub-path %q is not valid for key %q: %w", sub, key, err)
		}
		sub = clean
	}
	return ScopeValue{Space: space, Key: key, Sub: sub}, nil
}

// normalizeScopeSub normalizes the relative subpath under key: it must start with /, is
// validated segment by segment, and has its trailing slash stripped.
func normalizeScopeSub(sub string) (string, error) {
	if !strings.HasPrefix(sub, "/") {
		return "", fmt.Errorf("sub-path must start with /")
	}
	// Only a single trailing slash may be stripped ("photos/" is equivalent to "photos");
	// an empty segment in the middle is always rejected — a plain Trim would silently fold
	// "//x" into "/x", which is silent rewriting rather than validation.
	body := strings.TrimSuffix(sub, "/")
	if body == "" || body == "/" {
		return "", nil // "/" is equivalent to the whole key
	}
	segments := strings.Split(body[1:], "/")
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return "", fmt.Errorf("sub-path must not contain an empty segment")
		}
		if seg == "." || seg == ".." {
			return "", fmt.Errorf("sub-path must not contain \".\" or \"..\" segments")
		}
		for _, r := range seg {
			if r == 0 || unicode.IsControl(r) {
				return "", fmt.Errorf("sub-path must not contain control characters")
			}
		}
	}
	out := "/" + strings.Join(segments, "/")
	if len(out) > 1024 {
		return "", fmt.Errorf("sub-path is too long")
	}
	return out, nil
}

// Contains reports whether v's range covers other (component-wise comparison, design doc
// §3.4).
//
//	space axis: space.* ⊂ all.*; app is incomparable with the other two (different trees)
//	path axis: <key> ⊂ all paths; <key>/<sub> ⊂ <key>
func (v ScopeValue) Contains(other ScopeValue) bool {
	if v.IsZero() || other.IsZero() {
		return false
	}
	if v.Space != other.Space {
		return v.Space == ScopeSpaceAll && other.Space == ScopeSpaceOwn
	}
	if v.WholeSpace() {
		return true
	}
	if other.WholeSpace() || v.Key != other.Key {
		return false
	}
	return ScopeSubContains(v.Sub, other.Sub)
}

// ScopeSubContains compares subpaths by path segment: an empty a means the whole key.
func ScopeSubContains(a, b string) bool {
	a = strings.Trim(a, "/")
	b = strings.Trim(b, "/")
	switch {
	case a == b:
		return true
	case a == "":
		return true
	case b == "":
		return false
	default:
		return strings.HasPrefix(b, a+"/")
	}
}

// UFIPoolRel extracts the UFI's relative path inside the storage pool (`/Music/2024/a.mp3`).
//
// A UFI looks like `/ufi/pool-raw/<pool>/<rel...>`; the pool name varies between spaces
// (`{ns}_{pool}`), so only the pool-name segment is skipped and its exact value is never
// relied upon. Returns false when it cannot be extracted.
//
// The UFI shape produced by event producers is part of the platform contract, so this
// parsing also lives in the protocol layer: the daemon's fan-out filtering and the SDK's
// local filtering must understand "where this file is" by the same rules.
func UFIPoolRel(ufi string) (string, bool) {
	ufi = strings.TrimSpace(ufi)
	if ufi == "" {
		return "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(ufi, "/"), "/", 4)
	// parts: ["ufi", "pool-raw", "<pool>", "<rel...>"]
	if len(parts) < 3 || parts[0] != "ufi" || parts[1] != "pool-raw" {
		return "", false
	}
	if len(parts) == 3 || strings.Trim(parts[3], "/") == "" {
		return "/", true
	}
	return "/" + strings.Trim(parts[3], "/"), true
}

// PathWithin reports whether rel falls within want (compared on path segment boundaries).
//
// Segment boundaries are used rather than a bare prefix: `/Music2024` is not within `/Music`.
func PathWithin(want, rel string) bool {
	want = "/" + strings.Trim(want, "/")
	rel = "/" + strings.Trim(rel, "/")
	if want == "/" {
		return true
	}
	if rel == want {
		return true
	}
	return strings.HasPrefix(rel, want+"/")
}

// ScopeKeyForDir maps an in-pool relative path back to `<key>[/sub]`; returns false when it
// is not under any registered directory.
//
// Use case: an application's user interface still speaks in terms of "directories"
// (`/Music/2024`), while storage and filtering use scope values (`space.music/2024`). The
// reverse mapping must use the same key table and match the **longest** entry, or
// `/Music/2024` would be swallowed by a shorter match.
func ScopeKeyForDir(dir string) (string, bool) {
	dir = "/" + strings.Trim(strings.TrimSpace(dir), "/")
	if dir == "/" {
		return "", false
	}
	bestKey, bestDir := "", ""
	for key, keyDir := range scopeKeyDirs {
		if !PathWithin(keyDir, dir) {
			continue
		}
		// Longest match: if the key table ever contains a prefix relationship such as
		// `/Photo` and `/Photos`, the shorter one would swallow the longer one.
		if len(keyDir) > len(bestDir) {
			bestKey, bestDir = key, keyDir
		}
	}
	if bestKey == "" {
		return "", false
	}
	return bestKey + strings.TrimPrefix(dir, bestDir), true
}

// ScopeKeyPath returns the scope value's in-pool relative path (`/Music/2024`): it returns
// "/" for a whole space and false for an unknown key.
//
// This is the single expansion rule from "value → path prefix", used by both local
// application filtering and daemon fan-out filtering.
func (v ScopeValue) ScopeKeyPath() (string, bool) {
	if v.Space == ScopeSpaceApp {
		// The application private directory is in no storage pool, so there is no
		// "in-pool relative path" for it.
		return "", false
	}
	if v.WholeSpace() {
		return "/", true
	}
	dir, ok := ScopeKeyDir(v.Key)
	if !ok {
		return "", false
	}
	if sub := strings.Trim(v.Sub, "/"); sub != "" {
		return strings.TrimRight(dir, "/") + "/" + sub, true
	}
	return dir, true
}
