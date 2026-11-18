package appsdk

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	appfs "github.com/siyouyun-open/siyouyun_sdk/v2/fs"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
	"github.com/siyouyun-open/siyouyun_sdk/v2/runtime"
)

// writeRuntimeSpec writes a valid RuntimeSpec and returns its path.
func writeRuntimeSpec(t *testing.T, spec *runtime.Spec) string {
	t.Helper()
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	path := filepath.Join(t.TempDir(), "runtime.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

// minimalSpec builds a valid single-space spec with one scope mount.
func minimalSpec(t *testing.T) *runtime.Spec {
	t.Helper()
	dir := t.TempDir()
	music := filepath.Join(dir, "music")
	if err := os.MkdirAll(music, 0o755); err != nil {
		t.Fatal(err)
	}
	return &runtime.Spec{
		Version: runtime.Version,
		App: runtime.App{
			ID: "com.nas.sys.music", Code: "music", Version: "2.4.1",
			InstanceID: "inst-music", Namespace: "default",
		},
		Paths: runtime.Paths{
			App: filepath.Join(dir, "app"), Data: filepath.Join(dir, "data"),
			Runtime: filepath.Join(dir, "run"), Secrets: filepath.Join(dir, "run/secrets"),
			Cache: filepath.Join(dir, "cache"),
		},
		Bus:    runtime.Bus{Socket: filepath.Join(dir, "run/bus.sock"), HTTP: "http://127.0.0.1:8080"},
		Spaces: []string{"alice-main"},
		Mounts: []runtime.Mount{{
			Scope: "space.music", Space: "alice-main", Pool: "syspool", Root: "Music",
			Path: music, Mode: runtime.ModeReadOnly, GrantID: "grant_music",
		}},
		Grants: []runtime.Grant{{Scope: "space.music", Space: "alice-main", Ops: []string{runtime.OpRead}}},
	}
}

// FromEnv still reads the identity/bus bootstrap env, but the data plane comes
// exclusively from the RuntimeSpec.
func TestFromEnvLoadsRuntimeSpec(t *testing.T) {
	spec := minimalSpec(t)
	specPath := writeRuntimeSpec(t, spec)
	t.Setenv(runtime.EnvSpecPath, specPath)
	t.Setenv(protocol.EnvAppCode, "music")
	t.Setenv(protocol.EnvAppID, "com.nas.sys.music")
	t.Setenv(protocol.EnvInstanceID, "inst-env")
	t.Setenv(protocol.EnvNamespace, "default")
	t.Setenv(protocol.EnvRuntime, "native")
	t.Setenv(protocol.EnvEndpoint, "http://127.0.0.1:42001")
	t.Setenv(protocol.EnvPort, "42001")
	t.Setenv(protocol.EnvBusSocket, "/state/bus/app.sock")
	t.Setenv(protocol.EnvBusHTTP, "http://127.0.0.1:8080")
	t.Setenv(protocol.EnvBusWS, "ws://127.0.0.1:8080/api/bus/ws")
	t.Setenv(protocol.EnvCallerToken, "token-123")

	rt := FromEnv()

	if rt.Spec == nil || rt.SpecPath != specPath || rt.specErr != nil {
		t.Fatalf("runtime spec not loaded: %#v", rt)
	}
	if rt.AppID != "com.nas.sys.music" || rt.AppCode != "music" {
		t.Fatalf("unexpected identity: %#v", rt)
	}
	if rt.InstanceID != "inst-music" || rt.Namespace != "default" {
		t.Fatalf("spec identity must win over env: %#v", rt)
	}
	if rt.DataDir != spec.Paths.Data || rt.InstallRoot != spec.Paths.App {
		t.Fatalf("paths must come from the spec: %#v", rt)
	}
	if rt.BusSocket != spec.Bus.Socket || rt.BusHTTP != spec.Bus.HTTP {
		t.Fatalf("bus endpoints must come from the spec: %#v", rt)
	}
	if rt.Port != 42001 || !rt.Managed() || !rt.IsNative() {
		t.Fatalf("unexpected runtime: %#v", rt)
	}
	opts := rt.BusOptions()
	if opts.SocketPath == "" || opts.WSURL == "" || opts.CallerToken != "token-123" {
		t.Fatalf("unexpected bus options: %#v", opts)
	}
}

// Without a platform environment and without a spec it must not panic and must
// not pretend to be managed.
func TestFromEnvWithoutPlatform(t *testing.T) {
	for _, key := range []string{
		protocol.EnvAppCode, protocol.EnvAppID, protocol.EnvCode, protocol.EnvInstanceID,
		protocol.EnvNamespace, protocol.EnvRuntime, protocol.EnvEndpoint, protocol.EnvPort,
		protocol.EnvInstallRoot, protocol.EnvStaticRoot, protocol.EnvInstallSpace, protocol.EnvDataDir,
		protocol.EnvBusSocket, protocol.EnvBusHTTP, protocol.EnvBusWS, protocol.EnvCallerToken,
		protocol.EnvContainerRuntime, protocol.EnvContainerUsername, protocol.EnvLegacyNamespace,
		runtime.EnvSpecPath,
	} {
		t.Setenv(key, "")
	}

	rt := FromEnv()
	if rt.Managed() || rt.HasBus() {
		t.Fatalf("unmanaged runtime reported as managed: %#v", rt)
	}
	if rt.Spec != nil || !errors.Is(rt.specErr, runtime.ErrNoSpec) {
		t.Fatalf("spec should be unavailable: %#v", rt)
	}
	app := New()
	defer app.Close()
	if state := app.State(); state.Transport != "none" && state.Transport != "http" {
		t.Fatalf("unexpected state: %#v", state)
	}
	if _, err := app.FS(); !errors.Is(err, appfs.ErrNoSpec) {
		t.Fatalf("FS without spec = %v, want ErrNoSpec", err)
	}
}

// Container apps keep the container variable set; the spec is read from the same
// bootstrap path (SYY_RUNTIME_SPEC wins, container path otherwise).
func TestFromEnvParsesContainerRuntime(t *testing.T) {
	for _, key := range []string{
		protocol.EnvAppID, protocol.EnvCode, protocol.EnvInstanceID, protocol.EnvNamespace,
		protocol.EnvRuntime, protocol.EnvEndpoint, protocol.EnvPort, protocol.EnvInstallRoot,
		protocol.EnvStaticRoot, protocol.EnvInstallSpace, protocol.EnvDataDir,
		protocol.EnvBusSocket, protocol.EnvBusHTTP, protocol.EnvBusWS, protocol.EnvCallerToken,
		runtime.EnvSpecPath,
	} {
		t.Setenv(key, "")
	}
	t.Setenv(protocol.EnvContainerRuntime, protocol.RuntimeContainer)
	t.Setenv(protocol.EnvAppCode, "com.nas.demo.hello-kv")
	t.Setenv(protocol.EnvLegacyNamespace, "admin1-main")
	t.Setenv(protocol.EnvContainerUsername, "admin1")

	rt := FromEnv()
	if !rt.Managed() || !rt.IsContainer() || rt.IsNative() {
		t.Fatalf("container runtime not recognized: %#v", rt)
	}
	if rt.AppID != "com.nas.demo.hello-kv" || rt.AppCode != "com.nas.demo.hello-kv" {
		t.Fatalf("container app identity = %#v", rt)
	}
	if rt.BusSocket != protocol.EnvContainerBusSocket {
		t.Fatalf("container bus socket = %q", rt.BusSocket)
	}
	if scope := rt.DefaultScope(); scope.Username != "admin1" || scope.Namespace != "main" {
		t.Fatalf("DefaultScope = %+v", scope)
	}
}

// FS caches the table and ReloadFS picks up a regenerated spec.
func TestFSLoadsAndReloadsSpec(t *testing.T) {
	spec := minimalSpec(t)
	specPath := writeRuntimeSpec(t, spec)
	rt := Runtime{AppCode: "music", DataDir: spec.Paths.Data, SpecPath: specPath}
	app := NewWithOptions(rt)
	defer app.Close()
	table, err := app.FS()
	if err != nil {
		t.Fatalf("FS: %v", err)
	}
	if table.Source() != specPath || len(table.Mounts()) != 1 {
		t.Fatalf("unexpected table: source=%q mounts=%d", table.Source(), len(table.Mounts()))
	}

	// The runtime regenerates the spec: a second scope appears.
	extra := t.TempDir()
	spec.Mounts = append(spec.Mounts, runtime.Mount{
		Scope: "space", Space: "alice-main", Pool: "data1",
		Path: extra, Mode: runtime.ModeReadOnly, GrantID: "grant_space",
	})
	spec.Grants = append(spec.Grants, runtime.Grant{
		Scope: "space", Space: "alice-main", Ops: []string{runtime.OpRead},
	})
	raw, _ := json.Marshal(spec)
	if err := os.WriteFile(specPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	reloaded, err := app.ReloadFS()
	if err != nil {
		t.Fatalf("ReloadFS: %v", err)
	}
	if len(reloaded.Mounts()) != 2 {
		t.Fatalf("reload did not pick up the new spec: mounts=%d", len(reloaded.Mounts()))
	}
}

// NewWithOptions keeps the bus option overrides for local development: the
// resulting client exists and a failed call surfaces as an error instead of a
// panic (the client connects lazily).
func TestNewWithOptionsOverride(t *testing.T) {
	app := NewWithOptions(Runtime{}, WithBusSocket("/tmp/does-not-exist.sock"), WithCallerToken("t"))
	defer app.Close()
	if app.Bus() == nil || app.Capabilities() == nil {
		t.Fatal("bus client/capabilities must exist without a platform environment")
	}
}
