package appsdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

func TestFromEnvParsesRuntimeContract(t *testing.T) {
	t.Setenv(protocol.EnvAppCode, "album")
	t.Setenv(protocol.EnvAppID, "com.siyouyun.album")
	t.Setenv(protocol.EnvCode, "album")
	t.Setenv(protocol.EnvInstanceID, "native-album-0123456789abcdef")
	t.Setenv(protocol.EnvNamespace, "default")
	t.Setenv(protocol.EnvRuntime, "native")
	t.Setenv(protocol.EnvEndpoint, "http://127.0.0.1:42001")
	t.Setenv(protocol.EnvPort, "42001")
	t.Setenv(protocol.EnvInstallRoot, "/host-apps/album/install/1.0.0")
	t.Setenv(protocol.EnvStaticRoot, "/host-apps/album/install/1.0.0/static")
	t.Setenv(protocol.EnvInstallSpace, "main/syspool")
	t.Setenv(protocol.EnvDataDir, "/pool/.app/storage/com.siyouyun.album")
	t.Setenv(protocol.EnvBusSocket, "/state/bus/default/native-album-0123456789abcdef/app.sock")
	t.Setenv(protocol.EnvBusHTTP, "http://127.0.0.1:8080")
	t.Setenv(protocol.EnvBusWS, "ws://127.0.0.1:8080/api/bus/ws")
	t.Setenv(protocol.EnvCallerToken, "token-123")
	t.Setenv(protocol.EnvStoragePoolPrefix+"SYSPOOL", "/home/alice/main/syspool")
	t.Setenv(protocol.EnvStoragePoolPrefix+"MY_POOL_V2", "/home/alice/main/my-pool.v2")

	rt := FromEnv()

	if rt.AppID != "com.siyouyun.album" || rt.AppCode != "album" {
		t.Fatalf("unexpected app identity: %#v", rt)
	}
	if rt.InstanceID != "native-album-0123456789abcdef" || rt.Namespace != "default" {
		t.Fatalf("unexpected instance identity: %#v", rt)
	}
	if rt.Port != 42001 || rt.Endpoint != "http://127.0.0.1:42001" {
		t.Fatalf("unexpected endpoint: %#v", rt)
	}
	if rt.DataDir == "" || rt.InstallSpace != "main/syspool" {
		t.Fatalf("unexpected storage fields: %#v", rt)
	}
	if !rt.Managed() || !rt.HasBus() {
		t.Fatalf("runtime should be managed with a bus: %#v", rt)
	}

	// Pool names are looked up using daemon's normalization rules, so the original names
	// (including `.`/`-`) also match.
	if path, ok := rt.FindStoragePool("syspool"); !ok || path != "/home/alice/main/syspool" {
		t.Fatalf("syspool lookup = %q ok=%v", path, ok)
	}
	if path, ok := rt.FindStoragePool("my-pool.v2"); !ok || path != "/home/alice/main/my-pool.v2" {
		t.Fatalf("my-pool.v2 lookup = %q ok=%v", path, ok)
	}
	if _, ok := rt.FindStoragePool("missing"); ok {
		t.Fatal("missing pool must not resolve")
	}

	opts := rt.BusOptions()
	if opts.SocketPath == "" || opts.WSURL == "" || opts.HTTPURL == "" || opts.CallerToken != "token-123" {
		t.Fatalf("unexpected bus options: %#v", opts)
	}
}

// Without a platform environment (local development, unit tests) it must not panic and must
// not pretend to be managed: the app should still start and get a clear error when it actually
// calls a capability.
func TestFromEnvWithoutPlatform(t *testing.T) {
	for _, key := range []string{
		protocol.EnvAppCode, protocol.EnvAppID, protocol.EnvCode, protocol.EnvInstanceID,
		protocol.EnvNamespace, protocol.EnvRuntime, protocol.EnvEndpoint, protocol.EnvPort,
		protocol.EnvInstallRoot, protocol.EnvStaticRoot, protocol.EnvInstallSpace, protocol.EnvDataDir,
		protocol.EnvBusSocket, protocol.EnvBusHTTP, protocol.EnvBusWS, protocol.EnvCallerToken,
		protocol.EnvStoragePoolPrefix + "SYSPOOL",
		protocol.EnvContainerRuntime, protocol.EnvContainerUsername, protocol.EnvLegacyNamespace,
	} {
		t.Setenv(key, "")
	}

	rt := FromEnv()
	if rt.Managed() || rt.HasBus() {
		t.Fatalf("unmanaged runtime reported as managed: %#v", rt)
	}
	if rt.StoragePools != nil {
		t.Fatalf("no pools expected: %#v", rt.StoragePools)
	}

	app := New()
	defer app.Close()
	// Not connected yet, but there must be a readable state rather than a panic.
	if state := app.State(); state.Transport != "none" && state.Transport != "http" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

// Container apps get a different set of variables: no instance ID / caller token / install
// directory, and the bus socket and mount table live at fixed paths inside the container. After
// normalization Runtime's semantics must match native.
func TestFromEnvParsesContainerRuntime(t *testing.T) {
	for _, key := range []string{
		protocol.EnvAppID, protocol.EnvCode, protocol.EnvInstanceID, protocol.EnvNamespace,
		protocol.EnvRuntime, protocol.EnvEndpoint, protocol.EnvPort, protocol.EnvInstallRoot,
		protocol.EnvStaticRoot, protocol.EnvInstallSpace, protocol.EnvDataDir,
		protocol.EnvBusSocket, protocol.EnvBusHTTP, protocol.EnvBusWS, protocol.EnvCallerToken,
	} {
		t.Setenv(key, "")
	}
	t.Setenv(protocol.EnvContainerRuntime, protocol.RuntimeContainer)
	// Inside a container APPCODE holds the app id, not the install directory name.
	t.Setenv(protocol.EnvAppCode, "com.nas.demo.hello-kv")
	t.Setenv(protocol.EnvLegacyNamespace, "admin1-main")
	t.Setenv(protocol.EnvContainerUsername, "admin1")

	rt := FromEnv()

	if !rt.Managed() || !rt.IsContainer() || rt.IsNative() {
		t.Fatalf("container runtime not recognized: %#v", rt)
	}
	if rt.RuntimeKind != protocol.ContainerRuntimeKind {
		t.Fatalf("runtime kind = %q", rt.RuntimeKind)
	}
	if rt.AppID != "com.nas.demo.hello-kv" || rt.AppCode != "com.nas.demo.hello-kv" {
		t.Fatalf("unexpected app identity: %#v", rt)
	}
	if rt.Namespace != "admin1-main" || rt.Username != "admin1" {
		t.Fatalf("unexpected space: %#v", rt)
	}
	// The socket is part of the deployment contract and needs no environment variable.
	if rt.BusSocket != protocol.EnvContainerBusSocket {
		t.Fatalf("bus socket = %q, want %q", rt.BusSocket, protocol.EnvContainerBusSocket)
	}
	if !rt.HasBus() {
		t.Fatalf("container must have a bus entry: %#v", rt)
	}

	// The deployment space is admin1-main, but the scope wants username + **user space suffix**;
	// passing the full name would concatenate into admin1-admin1-main and be rejected by daemon.
	scope := rt.DefaultScope()
	if scope.Username != "admin1" || scope.Namespace != "main" || scope.GroupName != "" {
		t.Fatalf("unexpected default scope: %#v", scope)
	}
	// When the username and the space prefix do not match, leave namespace empty and let daemon
	// derive it from the connection.
	mismatch := Runtime{RuntimeKind: protocol.ContainerRuntimeKind, Username: "alice", Namespace: "admin1-main"}
	if got := mismatch.DefaultScope(); got.Username != "alice" || got.Namespace != "" {
		t.Fatalf("mismatched prefix must fall back to daemon derivation: %#v", got)
	}
	// native's default scope must be empty: it cannot derive the user and the caller must
	// provide it explicitly.
	native := Runtime{RuntimeKind: protocol.RuntimeNative, Namespace: protocol.DefaultNamespace}
	if s := native.DefaultScope(); !s.IsZero() {
		t.Fatalf("native default scope must be empty, got %#v", s)
	}
	// An explicitly injected MINI_APP_BUS_SOCKET takes priority over the container default
	// (convenient for local integration testing).
	t.Setenv(protocol.EnvBusSocket, "/tmp/custom.sock")
	if got := FromEnv().BusSocket; got != "/tmp/custom.sock" {
		t.Fatalf("explicit socket must win, got %q", got)
	}
}

// An empty-string storage pool path must not be treated as a valid pool (otherwise the app
// would get "" as a directory).
func TestReadStoragePoolsSkipsEmptyPaths(t *testing.T) {
	t.Setenv(protocol.EnvStoragePoolPrefix+"SYSPOOL", "")
	t.Setenv(protocol.EnvStoragePoolPrefix+"GOOD", "/home/alice/main/good")
	t.Setenv(protocol.EnvStoragePoolPrefix, "/should/be/ignored")

	pools := readStoragePools()
	if _, ok := pools["SYSPOOL"]; ok {
		t.Fatalf("empty path must be skipped: %#v", pools)
	}
	if pools["GOOD"] != "/home/alice/main/good" {
		t.Fatalf("unexpected pools: %#v", pools)
	}
	if len(pools) != 1 {
		t.Fatalf("prefix-only variable must be ignored: %#v", pools)
	}
}

func TestNormalizePoolName(t *testing.T) {
	cases := map[string]string{
		"syspool":    "SYSPOOL",
		"my-pool.v2": "MY_POOL_V2",
		"  spaced  ": "SPACED",
		"a/b":        "A_B",
	}
	for in, want := range cases {
		if got := normalizePoolName(in); got != want {
			t.Fatalf("normalizePoolName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Option overrides the bus address, which is convenient for local development/integration testing.
func TestNewWithOptionsOverride(t *testing.T) {
	rt := Runtime{RuntimeKind: protocol.RuntimeNative}
	app := NewWithOptions(rt, WithBusHTTP("http://127.0.0.1:9999"), WithCallerToken("t"))
	defer app.Close()

	if !app.Runtime.Managed() {
		t.Fatal("runtime with native kind must be managed")
	}
	if app.Capabilities() == nil || app.KV() == nil || app.Bus() == nil {
		t.Fatal("facades must be wired")
	}
}

func writeMountTableFile(t *testing.T, dir string, table protocol.MountTable) string {
	t.Helper()
	path := filepath.Join(dir, protocol.MountTableFileName)
	data, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// FS reads the mount table from MINI_APP_MOUNT_TABLE; ReloadFS lets a native shared instance
// see new directories after daemon refreshes the authorized spaces, without a restart.
func TestFSLoadsAndReloadsMountTable(t *testing.T) {
	base := t.TempDir()
	poolA := filepath.Join(base, "a")
	poolB := filepath.Join(base, "b")
	for _, dir := range []string{poolA, poolB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	path := writeMountTableFile(t, base, protocol.MountTable{
		Version: protocol.MountTableVersion,
		Pools:   []protocol.MountEntry{{Name: "syspool", Space: "alice-main", LocalPath: poolA, Writable: true}},
	})
	t.Setenv(protocol.EnvMountTable, path)

	app := New()
	defer app.Close()
	if app.Runtime.MountTable != path {
		t.Fatalf("Runtime.MountTable = %q, want %q", app.Runtime.MountTable, path)
	}

	table, err := app.FS()
	if err != nil {
		t.Fatalf("FS: %v", err)
	}
	if len(table.Pools()) != 1 || table.Pools()[0].Root() != poolA {
		t.Fatalf("unexpected pools: %#v", table.Pools())
	}

	// daemon refreshes the mount table (adding a pool for a newly authorized space).
	writeMountTableFile(t, base, protocol.MountTable{
		Version: protocol.MountTableVersion,
		Pools: []protocol.MountEntry{
			{Name: "syspool", Space: "alice-main", LocalPath: poolA, Writable: true},
			{Name: "syspool", Space: "bob-main", LocalPath: poolB, Writable: true},
		},
	})

	// Before a reload it is still the cached old table.
	cached, err := app.FS()
	if err != nil || len(cached.Pools()) != 1 {
		t.Fatalf("FS should be cached: %#v err=%v", cached.Pools(), err)
	}

	reloaded, err := app.ReloadFS()
	if err != nil {
		t.Fatalf("ReloadFS: %v", err)
	}
	if len(reloaded.Pools()) != 2 {
		t.Fatalf("ReloadFS did not pick up changes: %#v", reloaded.Pools())
	}
	if _, err := reloaded.PoolIn("bob-main", "syspool"); err != nil {
		t.Fatalf("PoolIn after reload: %v", err)
	}
}

// Without a mount table FS returns a recognizable error rather than panicking or pretending
// to succeed.
func TestFSWithoutMountTable(t *testing.T) {
	t.Setenv(protocol.EnvMountTable, filepath.Join(t.TempDir(), "missing.json"))

	app := New()
	defer app.Close()
	if _, err := app.FS(); err == nil {
		t.Fatal("FS without a mount table must fail")
	}
}
