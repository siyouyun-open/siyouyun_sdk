// Package fs provides unified local file access for native apps and container apps.
//
// What is unified is not the "path" but **logical addressing**: the runtime hands the
// application a RuntimeSpec v1 (see the runtime package) whose mounts map a logical
// scope to the absolute path this process can open directly, plus the
// space/pool/root metadata that lets a pool UFI be resolved back to that mount.
// Both native and container processes are therefore ordinary POSIX calls — no HTTP
// round trip, no server-side file handle, and mmap/sqlite/sendfile all work.
//
// The RuntimeSpec is the only data-plane source. There is no mounts.json, no
// MINI_APP_MOUNT_TABLE and no NATIVE_STORAGE_POOL_* fallback.
package fs

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
	"github.com/siyouyun-open/siyouyun_sdk/v2/runtime"
)

// Predictable errors. Callers check them with errors.Is.
var (
	// ErrNoSpec reports that no usable RuntimeSpec was available.
	ErrNoSpec = errors.New("fs: runtime spec not available")
	// ErrNoMount reports that the requested mount/pool/scope is not in the spec.
	ErrNoMount = errors.New("fs: mount not found")
	// ErrAmbiguous reports a pool mounted by several scopes/spaces; use the
	// space-qualified API.
	ErrAmbiguous = errors.New("fs: mount is ambiguous across spaces/scopes")
	// ErrBadUFI reports a malformed pool UFI.
	ErrBadUFI = errors.New("fs: invalid ufi")
	// ErrEscape reports a relative path that escapes the mount root.
	ErrEscape = errors.New("fs: path escapes the mount root")
	// ErrReadOnly reports a write on a read-only mount.
	ErrReadOnly = errors.New("fs: mount is read-only")
	// ErrNoGrant reports a mount without a matching grant in the same spec.
	ErrNoGrant = errors.New("fs: mount has no matching grant")
	// ErrGrantDenied reports a mount whose mode is not allowed by its grant.
	ErrGrantDenied = errors.New("fs: grant does not allow the mount mode")
	// ErrNoAppData reports that the spec has no usable paths.data directory.
	ErrNoAppData = errors.New("fs: app data directory is not configured")
)

// Table is the local data plane described by a RuntimeSpec v1.
type Table struct {
	spec   *runtime.Spec
	source string
	// appData is the application's private persistent directory (paths.data).
	// It is not grant-backed: the spec itself is the authorisation source.
	appData *Mount
	// mounts are the grant-backed scope mounts, in spec order.
	mounts []*Mount
}

// Mount is one grant-backed directory the current process can open directly.
// Paths given to the POSIX methods are relative to the mount (a leading "/" is
// accepted); the mount root itself is only exposed for troubleshooting.
type Mount struct {
	scope     string
	space     string
	pool      string
	scopeRoot string // pool-relative scope root, "" for the whole space
	grantID   string
	root      string
	realRoot  string
	writable  bool
	expiresAt int64 // Unix milliseconds (aligned with core/minictl); 0 means no expiry
}

// Load reads the RuntimeSpec for the current process and builds the table.
func Load() (*Table, error) {
	return LoadFrom(runtime.Path())
}

// LoadFrom reads the RuntimeSpec at specPath and builds the table.
func LoadFrom(specPath string) (*Table, error) {
	spec, err := runtime.LoadFrom(specPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoSpec, err)
	}
	table, err := New(spec)
	if err != nil {
		return nil, err
	}
	table.source = strings.TrimSpace(specPath)
	return table, nil
}

// New builds a table from an already parsed spec (tests and custom sources).
func New(spec *runtime.Spec) (*Table, error) {
	if spec == nil {
		return nil, ErrNoSpec
	}
	table := &Table{spec: spec}
	if data := strings.TrimSpace(spec.Paths.Data); data != "" {
		table.appData = newMount(&runtime.Mount{Path: data, Mode: runtime.ModeReadWrite})
	}
	for i := range spec.Mounts {
		mount := spec.Mounts[i]
		grant, err := spec.GrantIn(mount.Space, mount.Scope)
		if err != nil {
			return nil, fmt.Errorf("%w: scope %q in space %q", ErrNoGrant, mount.Scope, mount.Space)
		}
		required := runtime.OpRead
		if mount.Writable() {
			required = runtime.OpWrite
		}
		if !grant.Allows(required) {
			return nil, fmt.Errorf("%w: scope %q in space %q is %q but grant is %v",
				ErrGrantDenied, mount.Scope, mount.Space, mount.Mode, grant.Ops)
		}
		table.mounts = append(table.mounts, newMount(&mount))
	}
	return table, nil
}

// newMount builds a Mount from a runtime mount. A runtime.Mount with only
// Path/Mode set is used for the app-private data directory.
func newMount(spec *runtime.Mount) *Mount {
	root := strings.TrimSpace(spec.Path)
	mount := &Mount{
		scope:     strings.TrimSpace(spec.Scope),
		space:     strings.TrimSpace(spec.Space),
		pool:      strings.TrimSpace(spec.Pool),
		scopeRoot: strings.Trim(strings.TrimSpace(spec.Root), "/"),
		grantID:   strings.TrimSpace(spec.GrantID),
		root:      root,
		writable:  spec.Mode == runtime.ModeReadWrite,
		expiresAt: spec.ExpiresAt,
	}
	// The real root is used to prevent symlink escapes: the mount point itself may be a symlink.
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != "" {
		mount.realRoot = resolved
	} else {
		mount.realRoot = root
	}
	return mount
}

// Source returns the RuntimeSpec path (for troubleshooting).
func (t *Table) Source() string {
	if t == nil {
		return ""
	}
	return t.source
}

// Spec returns the RuntimeSpec the table was built from.
func (t *Table) Spec() *runtime.Spec {
	if t == nil {
		return nil
	}
	return t.spec
}

// AppData returns the application's private persistent directory (paths.data).
func (t *Table) AppData() (*Mount, error) {
	if t == nil || t.appData == nil {
		return nil, ErrNoAppData
	}
	return t.appData, nil
}

// Mounts returns the grant-backed mounts in spec order.
func (t *Table) Mounts() []*Mount {
	if t == nil {
		return nil
	}
	return append([]*Mount(nil), t.mounts...)
}

// Mount resolves a logical scope. When the same scope is mounted in several
// deployment spaces it reports ErrAmbiguous; use MountIn then.
func (t *Table) Mount(scope string) (*Mount, error) {
	if t == nil {
		return nil, ErrNoMount
	}
	scope = strings.TrimSpace(scope)
	var found *Mount
	for _, mount := range t.mounts {
		if mount.scope != scope {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%w: scope %q is mounted in several spaces", ErrAmbiguous, scope)
		}
		found = mount
	}
	if found == nil {
		return nil, fmt.Errorf("%w: scope %q", ErrNoMount, scope)
	}
	return found, nil
}

// MountIn resolves a logical scope inside one deployment space.
func (t *Table) MountIn(space, scope string) (*Mount, error) {
	if t == nil {
		return nil, ErrNoMount
	}
	space = strings.TrimSpace(space)
	scope = strings.TrimSpace(scope)
	for _, mount := range t.mounts {
		if mount.scope == scope && mount.space == space {
			return mount, nil
		}
	}
	return nil, fmt.Errorf("%w: scope %q in space %q", ErrNoMount, scope, space)
}

// Scopes returns the mounted logical scopes (deduplicated, spec order).
func (t *Table) Scopes() []string {
	if t == nil {
		return nil
	}
	scopes := make([]string, 0, len(t.mounts))
	seen := make(map[string]struct{}, len(t.mounts))
	for _, mount := range t.mounts {
		if _, ok := seen[mount.scope]; ok {
			continue
		}
		seen[mount.scope] = struct{}{}
		scopes = append(scopes, mount.scope)
	}
	return scopes
}

// Spaces returns the deployment spaces this instance is authorised to serve.
//
// It is the spec's spaces list; when the writer does not provide one it falls
// back to app.namespace (a container carries its deployment space there) and to
// the spaces carried by mounts. The result is sorted and deduplicated.
func (t *Table) Spaces() []string {
	if t == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(t.spec.Spaces)+1)
	add := func(space string) {
		space = strings.TrimSpace(space)
		if space == "" {
			return
		}
		if _, ok := seen[space]; ok {
			return
		}
		seen[space] = struct{}{}
	}
	for _, space := range t.spec.Spaces {
		add(space)
	}
	if ns := strings.TrimSpace(t.spec.App.Namespace); ns != "" && ns != protocol.DefaultNamespace {
		add(ns)
	}
	for _, mount := range t.mounts {
		add(mount.space)
	}
	spaces := make([]string, 0, len(seen))
	for space := range seen {
		spaces = append(spaces, space)
	}
	sort.Strings(spaces)
	return spaces
}

// HasSpace reports whether the deployment space is in the authorised set.
func (t *Table) HasSpace(space string) bool {
	space = strings.TrimSpace(space)
	if space == "" {
		return false
	}
	for _, candidate := range t.Spaces() {
		if candidate == space {
			return true
		}
	}
	return false
}

// SpaceMounts returns the mounts belonging to a deployment space.
func (t *Table) SpaceMounts(space string) []*Mount {
	if t == nil {
		return nil
	}
	space = strings.TrimSpace(space)
	out := make([]*Mount, 0, len(t.mounts))
	for _, mount := range t.mounts {
		if mount.space == space {
			out = append(out, mount)
		}
	}
	return out
}

// LocalPath resolves a pool UFI (/ufi/pool-raw/<pool>/<rel>) to a local absolute
// path using the space/pool/root metadata of the mounts. The unique matching
// mount is required; several matches report ErrAmbiguous.
func (t *Table) LocalPath(ufi string) (string, error) {
	return t.localPath("", ufi)
}

// LocalPathIn resolves a pool UFI within one deployment space.
func (t *Table) LocalPathIn(space, ufi string) (string, error) {
	return t.localPath(strings.TrimSpace(space), ufi)
}

// LocalPathForUGN fills in the deployment space from the UGN and resolves the UFI.
//
// A UFI itself does not contain the space, and several spaces may have pools with
// the same name; an event carries the UGN as well, so only together can the two
// uniquely determine a local path.
func (t *Table) LocalPathForUGN(username, groupName, namespace, ufi string) (string, error) {
	return t.LocalPathIn(DeploySpace(username, groupName, namespace), ufi)
}

// PoolResolve resolves a pool-relative path on the unique mount that advertises
// the pool in the given space (used by SpaceView).
func (t *Table) PoolResolve(space, pool, rel string) (string, error) {
	if t == nil {
		return "", ErrNoSpec
	}
	space = strings.TrimSpace(space)
	pool = strings.TrimSpace(pool)
	var found *Mount
	var foundRel string
	matches := 0
	for _, mount := range t.mounts {
		if mount.pool != pool {
			continue
		}
		if space != "" && mount.space != space {
			continue
		}
		scopeRel, ok := mount.scopeRelative(rel)
		if !ok {
			continue
		}
		found, foundRel = mount, scopeRel
		matches++
	}
	switch matches {
	case 0:
		return "", fmt.Errorf("%w: pool %q in space %q", ErrNoMount, pool, space)
	case 1:
		return found.Resolve(foundRel)
	default:
		return "", fmt.Errorf("%w: pool %q in space %q is mounted by several scopes", ErrAmbiguous, pool, space)
	}
}

func (t *Table) localPath(space, ufi string) (string, error) {
	if t == nil {
		return "", ErrNoSpec
	}
	pool, rel, err := ParseUFI(ufi)
	if err != nil {
		return "", err
	}
	var found *Mount
	var foundRel string
	matches := 0
	for _, mount := range t.mounts {
		if mount.pool != pool {
			continue
		}
		if space != "" && mount.space != "" && mount.space != space {
			continue
		}
		scopeRel, ok := mount.scopeRelative(rel)
		if !ok {
			continue
		}
		found, foundRel = mount, scopeRel
		matches++
	}
	switch matches {
	case 0:
		if space != "" && !t.HasSpace(space) {
			return "", fmt.Errorf("%w: space %q is not authorised", ErrNoMount, space)
		}
		return "", fmt.Errorf("%w: pool %q in space %q", ErrNoMount, pool, space)
	case 1:
		return found.Resolve(foundRel)
	default:
		return "", fmt.Errorf("%w: pool %q in space %q is mounted by several scopes", ErrAmbiguous, pool, space)
	}
}

// scopeRelative maps a pool-relative path (e.g. Music/2024/a.mp3) to the
// mount-relative path (e.g. 2024/a.mp3) using the mount's scope root.
func (m *Mount) scopeRelative(rel string) (string, bool) {
	rel = strings.Trim(rel, "/")
	if m.scopeRoot == "" {
		return rel, true
	}
	if rel == m.scopeRoot {
		return ".", true
	}
	if strings.HasPrefix(rel, m.scopeRoot+"/") {
		return strings.TrimPrefix(rel, m.scopeRoot+"/"), true
	}
	return "", false
}

// Scope / Space / Pool / ScopeRoot / GrantID / Root / ExpiresAt are the
// read-only properties of Mount.
func (m *Mount) Scope() string     { return m.scope }
func (m *Mount) Space() string     { return m.space }
func (m *Mount) Pool() string      { return m.pool }
func (m *Mount) ScopeRoot() string { return m.scopeRoot }
func (m *Mount) GrantID() string   { return m.grantID }
func (m *Mount) Root() string      { return m.root }
func (m *Mount) ExpiresAt() int64  { return m.expiresAt }
func (m *Mount) Writable() bool    { return m != nil && m.writable }

// Expired reports whether the grant behind the mount has expired at now.
//
// expiresAt is Unix **milliseconds** (the runtime daemon forwards core's
// millisecond grant expiry verbatim and its own expiry timers use ms); 0 means
// no expiry.
func (m *Mount) Expired(now time.Time) bool {
	if m == nil || m.expiresAt <= 0 {
		return false
	}
	return now.UnixMilli() >= m.expiresAt
}

// String is convenient for log and test output.
func (m *Mount) String() string {
	if m == nil {
		return "<nil mount>"
	}
	return fmt.Sprintf("mount{scope=%s space=%s pool=%s root=%s}", m.scope, m.space, m.pool, m.root)
}

// DeploySpace computes the deploy space identifier from the UGN.
//
// Consistent with the daemon's rules: personal space {username}-{namespace}, group space {groupname}-{namespace}.
func DeploySpace(username, groupName, namespace string) string {
	owner := strings.TrimSpace(username)
	if group := strings.TrimSpace(groupName); group != "" && group != owner {
		owner = group
	}
	namespace = strings.TrimSpace(namespace)
	switch {
	case owner == "":
		return namespace
	case namespace == "":
		return owner
	default:
		return owner + "-" + namespace
	}
}

// ParseUFI splits out the pool name and relative path of a UFI.
// Of the form /ufi/pool-raw/<pool>[/<rel>]; it returns ErrBadUFI when it cannot be parsed.
func ParseUFI(ufi string) (string, string, error) {
	raw := strings.TrimSpace(ufi)
	if !strings.HasPrefix(raw, protocol.UFIPoolPrefix) {
		return "", "", fmt.Errorf("%w: %s", ErrBadUFI, ufi)
	}
	rest := strings.TrimPrefix(raw, protocol.UFIPoolPrefix)
	if rest == "" {
		return "", "", fmt.Errorf("%w: %s", ErrBadUFI, ufi)
	}
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		pool := rest[:idx]
		rel := strings.TrimLeft(rest[idx+1:], "/")
		if pool == "" {
			return "", "", fmt.Errorf("%w: %s", ErrBadUFI, ufi)
		}
		return pool, rel, nil
	}
	return rest, "", nil
}

// UFI builds a UFI from a pool name and a relative path.
func UFI(pool, rel string) string {
	pool = strings.TrimSpace(pool)
	rel = strings.TrimLeft(strings.TrimSpace(rel), "/")
	if rel == "" {
		return protocol.UFIPoolPrefix + pool
	}
	return protocol.UFIPoolPrefix + pool + "/" + rel
}
