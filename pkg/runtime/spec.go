// Package runtime reads the SIYOUYUN RuntimeSpec v1 startup contract.
//
// The runtime daemon is the only writer of the spec; applications only read it.
// Native processes receive the spec path through the single SYY_RUNTIME_SPEC
// bootstrap variable, while containers always expose the spec at
// /run/syy/runtime.json. Applications must resolve data, cache, secrets, bus and
// authorised storage through this spec; they must not inspect legacy process
// environment variables such as MINI_APP_* or NATIVE_STORAGE_POOL_*.
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
	ContainerSpecPath = "/run/syy/runtime.json"
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
)

// Spec is the RuntimeSpec v1 document.
type Spec struct {
	Version int     `json:"version"`
	App     App     `json:"app"`
	Paths   Paths   `json:"paths"`
	Mounts  []Mount `json:"mounts"`
	Grants  []Grant `json:"grants"`

	// BusConfig is the schema's "bus" object. It cannot be named Bus because
	// Spec exposes Bus() as an accessor and Go forbids a field and method with
	// the same name.
	BusConfig Bus `json:"bus"`
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
	ExpiresAt int64  `json:"expiresAt,omitempty"` // Unix seconds; 0 means no expiry.
}

// Grant records the operations allowed for a logical scope.
type Grant struct {
	Scope     string   `json:"scope"`
	Ops       []string `json:"ops"`
	ExpiresAt int64    `json:"expiresAt,omitempty"` // Unix seconds; 0 means no expiry.
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

// Bus returns the runtime bus endpoints for this instance.
func (s *Spec) Bus() Bus { return s.BusConfig }

// Mount returns the mount registered for a logical scope. Unknown scopes return
// an error wrapping ErrUnknownScope; callers should treat a missing grant as a
// denied scope rather than a bug.
func (s *Spec) Mount(scope string) (Mount, error) {
	for _, mount := range s.Mounts {
		if mount.Scope == scope {
			return mount, nil
		}
	}
	return Mount{}, fmt.Errorf("%w: %q", ErrUnknownScope, scope)
}

// MountPath returns the logical path and mode for scope.
func (s *Spec) MountPath(scope string) (string, Mode, error) {
	mount, err := s.Mount(scope)
	if err != nil {
		return "", "", err
	}
	return mount.Path, mount.Mode, nil
}

// Scopes returns the mounted logical scopes in spec order.
func (s *Spec) Scopes() []string {
	scopes := make([]string, 0, len(s.Mounts))
	for _, mount := range s.Mounts {
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
	if err := validateRuntimePath("bus.socket", s.BusConfig.Socket); err != nil {
		return err
	}
	if err := validateBusHTTP(s.BusConfig.HTTP); err != nil {
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

func validateMounts(mounts []Mount) error {
	seenScope := make(map[string]struct{}, len(mounts))
	seenPath := make(map[string]struct{}, len(mounts))
	for i, mount := range mounts {
		if !scopePattern.MatchString(mount.Scope) {
			return invalid("mounts[%d].scope %q must match %s", i, mount.Scope, scopePattern)
		}
		if _, duplicate := seenScope[mount.Scope]; duplicate {
			return invalid("mounts[%d].scope %q is duplicated", i, mount.Scope)
		}
		seenScope[mount.Scope] = struct{}{}
		if err := validateRuntimePath(fmt.Sprintf("mounts[%d].path", i), mount.Path); err != nil {
			return err
		}
		// mounts.path is the actual mount target opened by the app: in containers
		// it is /mnt/..., while for native apps it is the host absolute path inside
		// the tmpfs pool. It must not be the root directory. The grant source is not
		// carried in the spec.
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
	}
	return nil
}

func validateGrants(grants []Grant) error {
	seenScope := make(map[string]struct{}, len(grants))
	for i, grant := range grants {
		if !scopePattern.MatchString(grant.Scope) {
			return invalid("grants[%d].scope %q must match %s", i, grant.Scope, scopePattern)
		}
		if _, duplicate := seenScope[grant.Scope]; duplicate {
			return invalid("grants[%d].scope %q is duplicated", i, grant.Scope)
		}
		seenScope[grant.Scope] = struct{}{}
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
