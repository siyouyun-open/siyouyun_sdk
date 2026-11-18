// Package runtime reads the SIYOUYUN RuntimeSpec v1 startup contract.
//
// The runtime daemon is the only writer of the spec; applications only read it.
// Native processes receive the spec path through the single SYY_RUNTIME_SPEC
// bootstrap variable, while containers expose the spec at the fixed path
// /lacx/run/runtime.json (minictl unified its runtime directories under /lacx;
// the value is part of the deployment contract and needs no environment
// variable). Applications must resolve data, cache, secrets, bus and authorised
// storage through this spec; they must not inspect legacy process environment
// variables such as MINI_APP_* or NATIVE_STORAGE_POOL_*.
//
// The JSON representation is kept in sync with
// mini-runtime/docs/specs/runtime-spec-v1.schema.json.
package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

const (
	// Version is the only RuntimeSpec schema version understood by this package.
	Version = 1

	// EnvSpecPath is the single bootstrap environment variable used by native
	// applications to locate their RuntimeSpec.
	EnvSpecPath = "SYY_RUNTIME_SPEC"

	// ContainerSpecPath is the fixed RuntimeSpec location inside a container.
	//
	// It mirrors the host contract (mini-runtime pkg/protocol.ContainerRuntimeDir
	// => /lacx/run, and fsprovider.ContainerSpecPath). Older drafts used
	// /run/syy/runtime.json, but the runtime daemon has written /lacx/run since
	// the "runtime dirs unified under /lacx" refactor; this constant is the one
	// the SDK actually reads, so it must match the daemon.
	ContainerSpecPath = "/lacx/run/runtime.json"
)

// Mount modes allowed by RuntimeSpec v1.
const (
	ModeReadOnly  Mode = "ro"
	ModeReadWrite Mode = "rw"
)

// Grant operations allowed by RuntimeSpec v1.
const (
	OpRead  = "read"
	OpWrite = "write"
)

// scopePattern matches the logical scope names allowed by the schema.
var scopePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// spacePattern matches deployment space names such as admin1-main / group-team.
var spacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Loader errors. Callers can test them with errors.Is.
var (
	// ErrNoSpec reports that no readable RuntimeSpec file was found.
	ErrNoSpec = errors.New("runtime: runtime spec not found")
	// ErrInvalidSpec reports a spec that does not satisfy the v1 contract.
	ErrInvalidSpec = errors.New("runtime: invalid runtime spec")
	// ErrUnsupportedVersion reports a missing or non-v1 version field.
	ErrUnsupportedVersion = errors.New("runtime: unsupported runtime spec version")
	// ErrUnknownScope reports a logical scope absent from the mounts list.
	ErrUnknownScope = errors.New("runtime: unknown mount scope")
	// ErrAmbiguousScope reports a logical scope mounted in several spaces without
	// the caller supplying the space (use MountIn/GrantIn).
	ErrAmbiguousScope = errors.New("runtime: mount scope is ambiguous across spaces")
)

// Spec is the RuntimeSpec v1 document.
type Spec struct {
	Version int     `json:"version"`
	App     App     `json:"app"`
	Paths   Paths   `json:"paths"`
	Mounts  []Mount `json:"mounts"`
	Grants  []Grant `json:"grants"`

	// Spaces lists the deployment spaces this instance is authorised to serve
	// (native shared instances install once and serve many user spaces). It is
	// an additive v1 extension: older writers omit it, readers then fall back to
	// app.namespace.
	Spaces []string `json:"spaces,omitempty"`

	// Bus is the schema's "bus" object (the runtime bus entry points).
	Bus Bus `json:"bus"`
}

// App identifies the running application instance. It is fixed by the runtime;
// applications must not override it from the environment.
type App struct {
	ID         string `json:"id"`
	Code       string `json:"code"`
	Version    string `json:"version"`
	InstanceID string `json:"instanceId"`
	Namespace  string `json:"namespace"`
}

// Paths holds the runtime-provided logical directories exposed to the app.
type Paths struct {
	App     string `json:"app"`
	Data    string `json:"data"`
	Runtime string `json:"runtime"`
	Secrets string `json:"secrets"`
	Cache   string `json:"cache"`
}

// Bus describes how the app reaches the runtime bus.
type Bus struct {
	Socket string `json:"socket"`
	HTTP   string `json:"http,omitempty"`
}

// Mount is one grant-backed storage mount visible to the app. Path is a
// runtime logical path, never a host path.
type Mount struct {
	Scope     string `json:"scope"`
	Path      string `json:"path"`
	Mode      Mode   `json:"mode"`
	GrantID   string `json:"grantId"`
	ExpiresAt int64  `json:"expiresAt,omitempty"` // Unix milliseconds (core/daemon unit); 0 means no expiry.

	// Space / Pool / Root are optional additive v1 fields used by multi-space
	// native instances. Together with Path they let the SDK map a pool UFI
	// (/ufi/pool-raw/<pool>/<rel>) back to this mount: Space is the deployment
	// space (e.g. admin1-main), Pool is the storage pool name, and Root is the
	// scope root inside that pool (e.g. "Music"; empty or "/" for the whole
	// space). The authorised source path itself never appears in the spec.
	//
	// They are empty for container mounts and for mounts that only carry an
	// already-resolved target path.
	Space string `json:"space,omitempty"`
	Pool  string `json:"pool,omitempty"`
	Root  string `json:"root,omitempty"`
}

// Grant records the operations allowed for a logical scope.
type Grant struct {
	Scope string `json:"scope"`
	// Space is the optional deployment space for multi-space instances; it
	// scopes the grant together with Scope (the same logical scope may have
	// different ops in different spaces). Empty for single-space writers.
	Space     string   `json:"space,omitempty"`
	Ops       []string `json:"ops"`
	ExpiresAt int64    `json:"expiresAt,omitempty"` // Unix milliseconds (core/daemon unit); 0 means no expiry.
}

// Mode is a mount access mode: "ro" or "rw".
type Mode string

// Valid reports whether the mode is one of the schema enum values.
func (m Mode) Valid() bool { return m == ModeReadOnly || m == ModeReadWrite }

// Writable reports whether the mount allows writes.
func (m Mount) Writable() bool { return m.Mode == ModeReadWrite }

// Allows reports whether the grant contains op (OpRead or OpWrite).
func (g Grant) Allows(op string) bool {
	for _, candidate := range g.Ops {
		if candidate == op {
			return true
		}
	}
	return false
}

// Path returns the RuntimeSpec location for the current process: the native
// bootstrap variable (SYY_RUNTIME_SPEC) wins, otherwise the fixed container path
// is used.
func Path() string {
	if specPath := strings.TrimSpace(os.Getenv(EnvSpecPath)); specPath != "" {
		return specPath
	}
	return ContainerSpecPath
}

// Load reads the RuntimeSpec for the current process.
//
// Native applications are bootstrapped with SYY_RUNTIME_SPEC; when it is set it
// takes precedence. Otherwise the container location /run/syy/runtime.json is
// used. No other environment variable is consulted, so legacy variables such as
// MINI_APP_* or NATIVE_STORAGE_POOL_* can never influence the result.
func Load() (*Spec, error) {
	return load(os.Getenv(EnvSpecPath), ContainerSpecPath)
}

// LoadFrom reads and validates the RuntimeSpec stored at specPath. It exists so
// callers and tests can inject the spec location.
func LoadFrom(specPath string) (*Spec, error) {
	specPath = strings.TrimSpace(specPath)
	if specPath == "" {
		return nil, fmt.Errorf("%w: empty spec path", ErrNoSpec)
	}
	data, err := os.ReadFile(specPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s does not exist", ErrNoSpec, specPath)
		}
		return nil, fmt.Errorf("runtime: read runtime spec %s: %w", specPath, err)
	}
	spec, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", specPath, err)
	}
	return spec, nil
}

// Parse decodes and validates a RuntimeSpec v1 document.
func Parse(data []byte) (*Spec, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var spec Spec
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("%w: decode json: %v", ErrInvalidSpec, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: unexpected trailing data after the json object", ErrInvalidSpec)
	}
	if err := spec.validate(); err != nil {
		return nil, err
	}
	return &spec, nil
}

// AppDir returns the read-only application package directory (/app).
func (s *Spec) AppDir() string { return s.Paths.App }

// DataDir returns the persistent private data directory (/data).
func (s *Spec) DataDir() string { return s.Paths.Data }

// CacheDir returns the disposable cache directory (/cache).
func (s *Spec) CacheDir() string { return s.Paths.Cache }

// RuntimeDir returns the runtime directory (/run/syy), which holds the spec.
func (s *Spec) RuntimeDir() string { return s.Paths.Runtime }

// SecretsDir returns the 0600 secrets directory (/run/syy/secrets).
func (s *Spec) SecretsDir() string { return s.Paths.Secrets }

// Mount returns the mount registered for a logical scope. Unknown scopes return
// an error wrapping ErrUnknownScope; when the same scope exists in several
// deployment spaces it returns ErrAmbiguousScope and callers must use MountIn.
func (s *Spec) Mount(scope string) (Mount, error) {
	var found Mount
	count := 0
	for _, mount := range s.Mounts {
		if mount.Scope == scope {
			found = mount
			count++
		}
	}
	if count == 0 {
		return Mount{}, fmt.Errorf("%w: %q", ErrUnknownScope, scope)
	}
	if count > 1 {
		return Mount{}, fmt.Errorf("%w: %q exists in several spaces; use MountIn", ErrAmbiguousScope, scope)
	}
	return found, nil
}

// MountIn returns the mount registered for (space, scope).
func (s *Spec) MountIn(space, scope string) (Mount, error) {
	space = strings.TrimSpace(space)
	for _, mount := range s.Mounts {
		if mount.Scope == scope && mount.Space == space {
			return mount, nil
		}
	}
	return Mount{}, fmt.Errorf("%w: %q in space %q", ErrUnknownScope, scope, space)
}

// Grant returns the grant registered for a logical scope. Unknown scopes return
// an error wrapping ErrUnknownScope; a mount without a grant must not be opened.
func (s *Spec) Grant(scope string) (Grant, error) {
	var found Grant
	count := 0
	for _, grant := range s.Grants {
		if grant.Scope == scope {
			found = grant
			count++
		}
	}
	if count == 0 {
		return Grant{}, fmt.Errorf("%w: %q", ErrUnknownScope, scope)
	}
	if count > 1 {
		return Grant{}, fmt.Errorf("%w: %q exists in several spaces; use GrantIn", ErrAmbiguousScope, scope)
	}
	return found, nil
}

// GrantIn returns the grant registered for (space, scope). It first looks for a
// space-qualified grant, then falls back to a space-less grant for the scope.
func (s *Spec) GrantIn(space, scope string) (Grant, error) {
	space = strings.TrimSpace(space)
	for _, grant := range s.Grants {
		if grant.Scope == scope && grant.Space == space {
			return grant, nil
		}
	}
	if space != "" {
		for _, grant := range s.Grants {
			if grant.Scope == scope && grant.Space == "" {
				return grant, nil
			}
		}
	}
	return Grant{}, fmt.Errorf("%w: %q in space %q", ErrUnknownScope, scope, space)
}

// MountPath returns the logical path and mode for scope.
func (s *Spec) MountPath(scope string) (string, Mode, error) {
	mount, err := s.Mount(scope)
	if err != nil {
		return "", "", err
	}
	return mount.Path, mount.Mode, nil
}

// Scopes returns the mounted logical scopes in spec order, deduplicated (the
// same scope can be mounted once per deployment space).
func (s *Spec) Scopes() []string {
	scopes := make([]string, 0, len(s.Mounts))
	seen := make(map[string]struct{}, len(s.Mounts))
	for _, mount := range s.Mounts {
		if _, ok := seen[mount.Scope]; ok {
			continue
		}
		seen[mount.Scope] = struct{}{}
		scopes = append(scopes, mount.Scope)
	}
	return scopes
}

// load resolves the spec path: the native bootstrap variable wins, otherwise the
// fixed container path is used.
func load(envPath, containerPath string) (*Spec, error) {
	if specPath := strings.TrimSpace(envPath); specPath != "" {
		return LoadFrom(specPath)
	}
	if strings.TrimSpace(containerPath) == "" {
		return nil, fmt.Errorf("%w: no spec path configured", ErrNoSpec)
	}
	return LoadFrom(containerPath)
}

func (s *Spec) validate() error {
	if s.Version != Version {
		return fmt.Errorf("%w: got version %d, want %d", ErrUnsupportedVersion, s.Version, Version)
	}
	if err := validateApp(s.App); err != nil {
		return err
	}
	if err := validatePaths(s.Paths); err != nil {
		return err
	}
	if err := validateRuntimePath("bus.socket", s.Bus.Socket); err != nil {
		return err
	}
	if err := validateBusHTTP(s.Bus.HTTP); err != nil {
		return err
	}
	if err := validateSpaces(s.Spaces); err != nil {
		return err
	}
	if err := validateMounts(s.Mounts); err != nil {
		return err
	}
	return validateGrants(s.Grants)
}

func validateApp(app App) error {
	fields := []struct {
		name  string
		value string
	}{
		{"app.id", app.ID},
		{"app.code", app.Code},
		{"app.version", app.Version},
		{"app.instanceId", app.InstanceID},
		{"app.namespace", app.Namespace},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			return invalid("%s must not be empty", field.name)
		}
	}
	return nil
}

func validatePaths(paths Paths) error {
	fields := []struct {
		name  string
		value string
	}{
		{"paths.app", paths.App},
		{"paths.data", paths.Data},
		{"paths.runtime", paths.Runtime},
		{"paths.secrets", paths.Secrets},
		{"paths.cache", paths.Cache},
	}
	for _, field := range fields {
		if err := validateRuntimePath(field.name, field.value); err != nil {
			return err
		}
	}
	return nil
}

// validateSpaces checks the optional multi-space authorisation list.
func validateSpaces(spaces []string) error {
	seen := make(map[string]struct{}, len(spaces))
	for i, raw := range spaces {
		space := strings.TrimSpace(raw)
		if space == "" {
			return invalid("spaces[%d] must not be empty", i)
		}
		if space != raw {
			return invalid("spaces[%d] %q must not have surrounding whitespace", i, raw)
		}
		if !spacePattern.MatchString(space) {
			return invalid("spaces[%d] %q must match %s", i, space, spacePattern)
		}
		if _, duplicate := seen[space]; duplicate {
			return invalid("spaces[%d] %q is duplicated", i, space)
		}
		seen[space] = struct{}{}
	}
	return nil
}

func validateMounts(mounts []Mount) error {
	// Multi-space instances mount the same logical scope once per deployment
	// space, so uniqueness is on the (space, scope) pair (not scope alone).
	seenScope := make(map[string]struct{}, len(mounts))
	seenPath := make(map[string]struct{}, len(mounts))
	for i, mount := range mounts {
		if !scopePattern.MatchString(mount.Scope) {
			return invalid("mounts[%d].scope %q must match %s", i, mount.Scope, scopePattern)
		}
		space := strings.TrimSpace(mount.Space)
		if mount.Space != space {
			return invalid("mounts[%d].space %q must not have surrounding whitespace", i, mount.Space)
		}
		if space != "" && !spacePattern.MatchString(space) {
			return invalid("mounts[%d].space %q must match %s", i, space, spacePattern)
		}
		scopeKey := space + "\x00" + mount.Scope
		if _, duplicate := seenScope[scopeKey]; duplicate {
			return invalid("mounts[%d] scope %q in space %q is duplicated", i, mount.Scope, space)
		}
		seenScope[scopeKey] = struct{}{}
		if err := validateRuntimePath(fmt.Sprintf("mounts[%d].path", i), mount.Path); err != nil {
			return err
		}
		// mounts.path 是应用实际打开的挂载目标：容器是 /mnt/...，native 是
		// tmpfs 池里的宿主绝对路径；它不能是根目录。授权 source 不在 spec 中。
		if mount.Path == "/" {
			return invalid("mounts[%d].path must not be the host root", i)
		}
		if _, duplicate := seenPath[mount.Path]; duplicate {
			return invalid("mounts[%d].path %q is duplicated", i, mount.Path)
		}
		seenPath[mount.Path] = struct{}{}
		if !mount.Mode.Valid() {
			return invalid("mounts[%d].mode %q must be %q or %q", i, mount.Mode, ModeReadOnly, ModeReadWrite)
		}
		if strings.TrimSpace(mount.GrantID) == "" {
			return invalid("mounts[%d].grantId must not be empty", i)
		}
		if mount.ExpiresAt < 0 {
			return invalid("mounts[%d].expiresAt must not be negative", i)
		}
		if err := validateMountUFIOrigin(i, mount); err != nil {
			return err
		}
	}
	return nil
}

// validateMountUFIOrigin checks the optional space/pool/root triple that lets the
// SDK map pool UFIs back to this mount. All three are optional together, but once
// pool is present the space must be present too (otherwise the pool name is
// ambiguous across spaces), and root must be a clean pool-relative prefix.
func validateMountUFIOrigin(i int, mount Mount) error {
	space := strings.TrimSpace(mount.Space)
	pool := strings.TrimSpace(mount.Pool)
	root := strings.TrimSpace(mount.Root)
	if pool == "" && root == "" {
		return nil
	}
	if space == "" {
		return invalid("mounts[%d].space must not be empty when pool/root is set", i)
	}
	if pool == "" {
		return invalid("mounts[%d].pool must not be empty when root is set", i)
	}
	if pool != mount.Pool {
		return invalid("mounts[%d].pool %q must not have surrounding whitespace", i, mount.Pool)
	}
	if root == "" {
		return nil
	}
	normalized := strings.Trim(root, "/")
	if normalized == "" {
		// "/" is the whole-space root; treat it like an empty root.
		return nil
	}
	if strings.ContainsRune(normalized, 0) {
		return invalid("mounts[%d].root must not contain NUL", i)
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return invalid("mounts[%d].root %q must be a clean pool-relative path", i, mount.Root)
		}
	}
	return nil
}

func validateGrants(grants []Grant) error {
	seenScope := make(map[string]struct{}, len(grants))
	for i, grant := range grants {
		if !scopePattern.MatchString(grant.Scope) {
			return invalid("grants[%d].scope %q must match %s", i, grant.Scope, scopePattern)
		}
		space := strings.TrimSpace(grant.Space)
		if grant.Space != space {
			return invalid("grants[%d].space %q must not have surrounding whitespace", i, grant.Space)
		}
		if space != "" && !spacePattern.MatchString(space) {
			return invalid("grants[%d].space %q must match %s", i, space, spacePattern)
		}
		scopeKey := space + "\x00" + grant.Scope
		if _, duplicate := seenScope[scopeKey]; duplicate {
			return invalid("grants[%d] scope %q in space %q is duplicated", i, grant.Scope, space)
		}
		seenScope[scopeKey] = struct{}{}
		if len(grant.Ops) == 0 {
			return invalid("grants[%d].ops must not be empty", i)
		}
		seenOp := make(map[string]struct{}, len(grant.Ops))
		for j, op := range grant.Ops {
			if op != OpRead && op != OpWrite {
				return invalid("grants[%d].ops[%d] %q must be %q or %q", i, j, op, OpRead, OpWrite)
			}
			if _, duplicate := seenOp[op]; duplicate {
				return invalid("grants[%d].ops[%d] %q is duplicated", i, j, op)
			}
			seenOp[op] = struct{}{}
		}
		if grant.ExpiresAt < 0 {
			return invalid("grants[%d].expiresAt must not be negative", i)
		}
	}
	return nil
}

// validateRuntimePath checks a runtime logical path: non-empty, NUL-free,
// absolute and clean. Rejecting non-clean paths prevents "..", duplicate
// separators and trailing slashes from smuggling in host paths.
func validateRuntimePath(field, p string) error {
	if strings.TrimSpace(p) == "" {
		return invalid("%s must not be empty", field)
	}
	if strings.ContainsRune(p, 0) {
		return invalid("%s must not contain NUL", field)
	}
	if !path.IsAbs(p) {
		return invalid("%s must be an absolute path, got %q", field, p)
	}
	if path.Clean(p) != p {
		return invalid("%s must be a clean absolute path, got %q", field, p)
	}
	return nil
}

// validateBusHTTP validates the optional bus HTTP endpoint. An empty value is
// allowed for UDS-only deployments.
func validateBusHTTP(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return invalid("bus.http is not a valid URL: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return invalid("bus.http must be an http(s) URL, got %q", raw)
	}
	if u.Host == "" {
		return invalid("bus.http must contain a host, got %q", raw)
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSpec, fmt.Sprintf(format, args...))
}
