package fs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/siyouyun-open/siyouyun_sdk/v2/runtime"
)

// testSpec builds a multi-space native RuntimeSpec with real temp directories:
//
//	admin1-main: scope space.music -> <admin>/fuse/music, pool syspool, root Music
//	alice-main:  scope space.music -> <alice>/fuse/music, pool syspool, root Music
//	admin1-main: scope space       -> <admin>/fuse/space, pool data1, root "" (whole space)
func testSpec(t *testing.T) (*runtime.Spec, string, string) {
	t.Helper()
	base := t.TempDir()
	admin := filepath.Join(base, "admin")
	alice := filepath.Join(base, "alice")
	adminSpace := filepath.Join(base, "admin-space")
	for _, dir := range []string{admin, alice, adminSpace} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	spec := &runtime.Spec{
		Version: runtime.Version,
		App: runtime.App{
			ID: "com.nas.sys.music", Code: "music", Version: "2.4.1",
			InstanceID: "inst-music", Namespace: "default",
		},
		Paths: runtime.Paths{
			App: "/app", Data: filepath.Join(base, "data"), Runtime: "/run/lacx",
			Secrets: "/run/lacx/secrets", Cache: filepath.Join(base, "cache"),
		},
		Bus:    runtime.Bus{Socket: "/run/lacx/bus.sock"},
		Spaces: []string{"admin1-main", "alice-main"},
		Mounts: []runtime.Mount{
			{Scope: "space.music", Space: "admin1-main", Pool: "syspool", Root: "Music",
				Path: admin, Mode: runtime.ModeReadOnly, GrantID: "grant_admin_music"},
			{Scope: "space.music", Space: "alice-main", Pool: "syspool", Root: "Music",
				Path: alice, Mode: runtime.ModeReadWrite, GrantID: "grant_alice_music"},
			{Scope: "space", Space: "admin1-main", Pool: "data1", Root: "",
				Path: adminSpace, Mode: runtime.ModeReadOnly, GrantID: "grant_admin_space"},
		},
		Grants: []runtime.Grant{
			{Scope: "space.music", Space: "admin1-main", Ops: []string{runtime.OpRead}},
			{Scope: "space.music", Space: "alice-main", Ops: []string{runtime.OpRead, runtime.OpWrite}},
			{Scope: "space", Space: "admin1-main", Ops: []string{runtime.OpRead}},
		},
	}
	return spec, admin, alice
}

func TestParseAndBuildUFI(t *testing.T) {
	pool, rel, err := ParseUFI("/ufi/pool-raw/syspool/photos/a.jpg")
	if err != nil || pool != "syspool" || rel != "photos/a.jpg" {
		t.Fatalf("ParseUFI = (%q,%q,%v)", pool, rel, err)
	}
	pool, rel, err = ParseUFI("/ufi/pool-raw/syspool")
	if err != nil || pool != "syspool" || rel != "" {
		t.Fatalf("pool-only UFI = (%q,%q,%v)", pool, rel, err)
	}
	for _, bad := range []string{"", "syspool/a", "/ufi/pool-raw/", "/ufi/pool-raw"} {
		if _, _, err := ParseUFI(bad); !errors.Is(err, ErrBadUFI) {
			t.Fatalf("ParseUFI(%q) should fail, got %v", bad, err)
		}
	}
	if got := UFI("syspool", "/photos/a.jpg"); got != "/ufi/pool-raw/syspool/photos/a.jpg" {
		t.Fatalf("UFI = %q", got)
	}
	if got := UFI("syspool", ""); got != "/ufi/pool-raw/syspool" {
		t.Fatalf("UFI(pool only) = %q", got)
	}
}

func TestDeploySpace(t *testing.T) {
	cases := []struct {
		username, group, ns, want string
	}{
		{"alice", "alice", "main", "alice-main"},
		{"alice", "", "private", "alice-private"},
		{"alice", "group", "team", "group-team"},
		{"alice", "group", "", "group"},
		{"", "", "main", "main"},
	}
	for _, tc := range cases {
		if got := DeploySpace(tc.username, tc.group, tc.ns); got != tc.want {
			t.Fatalf("DeploySpace(%q,%q,%q) = %q, want %q", tc.username, tc.group, tc.ns, got, tc.want)
		}
	}
}

func TestTableFromRuntimeSpec(t *testing.T) {
	spec, admin, alice := testSpec(t)
	table, err := New(spec)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := table.Scopes(), []string{"space.music", "space"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Scopes = %v, want %v", got, want)
	}
	if got, want := table.Spaces(), []string{"admin1-main", "alice-main"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Spaces = %v, want %v", got, want)
	}
	if !table.HasSpace("admin1-main") || table.HasSpace("bob-main") || table.HasSpace("") {
		t.Fatalf("HasSpace is wrong")
	}
	// A scope mounted in several spaces is ambiguous without the space.
	if _, err := table.Mount("space.music"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("Mount(space.music) = %v, want ErrAmbiguous", err)
	}
	mount, err := table.MountIn("alice-main", "space.music")
	if err != nil || mount.Root() != alice || !mount.Writable() || mount.Pool() != "syspool" || mount.ScopeRoot() != "Music" {
		t.Fatalf("MountIn(alice) = %+v err=%v", mount, err)
	}
	adminMount, err := table.MountIn("admin1-main", "space.music")
	if err != nil || adminMount.Root() != admin || adminMount.Writable() {
		t.Fatalf("MountIn(admin) = %+v err=%v", adminMount, err)
	}
	appData, err := table.AppData()
	if err != nil || appData.Root() != spec.Paths.Data || !appData.Writable() {
		t.Fatalf("AppData = %+v err=%v", appData, err)
	}
}

// Pool UFIs resolve through space+pool+root to the matching mount.
func TestLocalPathForUFI(t *testing.T) {
	spec, admin, alice := testSpec(t)
	table, err := New(spec)
	if err != nil {
		t.Fatal(err)
	}
	local, err := table.LocalPathIn("admin1-main", "/ufi/pool-raw/syspool/Music/2024/a.mp3")
	if err != nil || local != filepath.Join(admin, "2024/a.mp3") {
		t.Fatalf("admin LocalPathIn = %q err=%v", local, err)
	}
	local, err = table.LocalPathIn("alice-main", "/ufi/pool-raw/syspool/Music/2024/a.mp3")
	if err != nil || local != filepath.Join(alice, "2024/a.mp3") {
		t.Fatalf("alice LocalPathIn = %q err=%v", local, err)
	}
	// Whole-space mount: pool root maps directly.
	local, err = table.LocalPathIn("admin1-main", "/ufi/pool-raw/data1/Documents/a.pdf")
	if err != nil || local != filepath.Join(filepath.Dir(admin), "admin-space", "Documents/a.pdf") {
		t.Fatalf("whole-space LocalPathIn = %q err=%v", local, err)
	}
	// UGN → space → UFI.
	if _, err := table.LocalPathForUGN("alice", "alice", "main", "/ufi/pool-raw/syspool/Music/a.mp3"); err != nil {
		t.Fatalf("LocalPathForUGN: %v", err)
	}
	// Without a space the same pool/root exists in two spaces → ambiguous.
	if _, err := table.LocalPath("/ufi/pool-raw/syspool/Music/2024/a.mp3"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("LocalPath without space = %v, want ErrAmbiguous", err)
	}
	// Unauthorised space.
	if _, err := table.LocalPathIn("bob-main", "/ufi/pool-raw/syspool/Music/a.mp3"); !errors.Is(err, ErrNoMount) {
		t.Fatalf("bob LocalPathIn = %v, want ErrNoMount", err)
	}
	// Unknown pool.
	if _, err := table.LocalPathIn("admin1-main", "/ufi/pool-raw/other/Music/a.mp3"); !errors.Is(err, ErrNoMount) {
		t.Fatalf("unknown pool = %v, want ErrNoMount", err)
	}
	// Pool path outside the scope root.
	if _, err := table.LocalPathIn("admin1-main", "/ufi/pool-raw/syspool/Music2/a.mp3"); !errors.Is(err, ErrNoMount) {
		t.Fatalf("outside root = %v, want ErrNoMount", err)
	}
	// Escapes are blocked after resolution.
	if _, err := table.LocalPathIn("admin1-main", "/ufi/pool-raw/syspool/Music/../../etc/passwd"); !errors.Is(err, ErrEscape) && !errors.Is(err, ErrNoMount) {
		t.Fatalf("escape = %v, want ErrEscape/ErrNoMount", err)
	}
	// Bad UFI.
	if _, err := table.LocalPathIn("admin1-main", "nope"); !errors.Is(err, ErrBadUFI) {
		t.Fatalf("bad ufi = %v, want ErrBadUFI", err)
	}
}

func TestTableGrantValidation(t *testing.T) {
	spec, _, _ := testSpec(t)
	// Mount without a matching grant.
	noGrant := *spec
	noGrant.Grants = nil
	if _, err := New(&noGrant); !errors.Is(err, ErrNoGrant) {
		t.Fatalf("no grant = %v, want ErrNoGrant", err)
	}
	// RW mount with a read-only grant.
	denied := *spec
	denied.Mounts = []runtime.Mount{spec.Mounts[1]}
	denied.Grants = []runtime.Grant{{Scope: "space.music", Space: "alice-main", Ops: []string{runtime.OpRead}}}
	if _, err := New(&denied); !errors.Is(err, ErrGrantDenied) {
		t.Fatalf("rw with read grant = %v, want ErrGrantDenied", err)
	}
}

func TestResolveBlocksEscapes(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "pool")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{root, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	mount := newMount(&runtime.Mount{Scope: "p", Path: root, Mode: runtime.ModeReadWrite})
	for _, rel := range []string{"..", "../outside/secret", "a/../../outside", "/../../etc/passwd", "link/secret"} {
		if _, err := mount.Resolve(rel); !errors.Is(err, ErrEscape) {
			t.Fatalf("Resolve(%q) must be rejected, got %v", rel, err)
		}
	}
	for _, rel := range []string{"", ".", "a/b.txt", "/a/b.txt", "a/../b.txt"} {
		if _, err := mount.Resolve(rel); err != nil {
			t.Fatalf("Resolve(%q) = %v", rel, err)
		}
	}
	if err := mount.RemoveAll("link"); err != nil {
		t.Fatalf("RemoveAll(link) = %v", err)
	}
}

// expiresAt in the RuntimeSpec is Unix milliseconds (core/daemon unit), not seconds.
func TestMountExpiryUsesUnixMilliseconds(t *testing.T) {
	mount := newMount(&runtime.Mount{
		Scope: "p", Path: t.TempDir(), Mode: runtime.ModeReadOnly,
		ExpiresAt: 1893456000000, // 2030-01-01T00:00:00Z in ms
	})
	now := time.Date(2029, 12, 31, 23, 59, 59, 0, time.UTC)
	if mount.Expired(now) {
		t.Fatal("mount must not be expired one second before expiresAt")
	}
	if !mount.Expired(time.UnixMilli(1893456000000)) {
		t.Fatal("mount must be expired exactly at expiresAt (ms)")
	}
	// A seconds-based timestamp would expire around now; the ms value must not.
	if mount.Expired(time.Unix(1893456000, 0)) != mount.Expired(time.UnixMilli(1893456000000)) {
		t.Fatalf("expiry must use UnixMilli consistently")
	}
	forever := newMount(&runtime.Mount{Scope: "p", Path: t.TempDir()})
	if forever.Expired(time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("expiresAt=0 must never expire")
	}
}

func TestMountPOSIXOps(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pool")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	mount := newMount(&runtime.Mount{Scope: "p", Path: root, Mode: runtime.ModeReadWrite})
	if err := mount.MkdirAll("photos/2026", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := mount.WriteFile("photos/2026/a.txt", []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := mount.ReadFile("photos/2026/a.txt")
	if err != nil || string(data) != "hello" {
		t.Fatalf("ReadFile = %q err=%v", data, err)
	}
	entries, err := mount.ReadDir("photos")
	if err != nil || len(entries) != 1 || entries[0].Name() != "2026" {
		t.Fatalf("ReadDir = %#v err=%v", entries, err)
	}
	if ok, err := mount.Exists("photos/2026/a.txt"); err != nil || !ok {
		t.Fatalf("Exists = %v err=%v", ok, err)
	}
	if ok, err := mount.Exists("nope"); err != nil || ok {
		t.Fatalf("Exists(nope) = %v err=%v", ok, err)
	}
	if err := mount.Remove("photos/2026/a.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestReadOnlyMountRejectsWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pool")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mount := newMount(&runtime.Mount{Scope: "p", Path: root, Mode: runtime.ModeReadOnly})
	if _, err := mount.ReadFile("existing"); err != nil {
		t.Fatalf("read on read-only mount must work: %v", err)
	}
	if _, err := mount.ReadDir(""); err != nil {
		t.Fatalf("ReadDir on read-only mount must work: %v", err)
	}
	if err := mount.WriteFile("new", []byte("x"), 0o644); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("WriteFile = %v, want ErrReadOnly", err)
	}
	if _, err := mount.Create("new"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Create = %v, want ErrReadOnly", err)
	}
	if _, err := mount.OpenFile("existing", os.O_RDWR, 0o644); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("OpenFile(O_RDWR) = %v, want ErrReadOnly", err)
	}
	if _, err := mount.Open("existing"); err != nil {
		t.Fatalf("Open(read-only) = %v", err)
	}
}

// Each export inside the container is an independent bind mount, so a cross-pool rename inevitably returns EXDEV; it must fall back to copy + delete.
func TestCrossDeviceMoveFallsBackToCopy(t *testing.T) {
	base := t.TempDir()
	srcRoot := filepath.Join(base, "src")
	dstRoot := filepath.Join(base, "dst")
	for _, dir := range []string{srcRoot, dstRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	src := newMount(&runtime.Mount{Scope: "src", Path: srcRoot, Mode: runtime.ModeReadWrite})
	dst := newMount(&runtime.Mount{Scope: "dst", Path: dstRoot, Mode: runtime.ModeReadWrite})
	if err := src.MkdirAll("dir/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := src.WriteFile("dir/a.txt", []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := src.WriteFile("dir/sub/b.txt", []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(srcRoot, "dir", "link")); err != nil {
		t.Fatal(err)
	}

	original := renameFn
	renameFn = func(_, _ string) error { return &os.LinkError{Op: "rename", Err: syscall.EXDEV} }
	t.Cleanup(func() { renameFn = original })

	if err := Move(src, "dir", dst, "moved"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(srcRoot, "dir")); !os.IsNotExist(err) {
		t.Fatalf("source should be removed, stat err=%v", err)
	}
	for rel, want := range map[string]string{"moved/a.txt": "A", "moved/sub/b.txt": "B"} {
		data, err := dst.ReadFile(rel)
		if err != nil || string(data) != want {
			t.Fatalf("dst %s = %q err=%v", rel, data, err)
		}
	}
	target, err := os.Readlink(filepath.Join(dstRoot, "moved", "link"))
	if err != nil || target != "a.txt" {
		t.Fatalf("symlink not preserved: %q err=%v", target, err)
	}
}

// A move within the same mount uses native rename and does not copy.
func TestMoveSameMountUsesRename(t *testing.T) {
	root := filepath.Join(t.TempDir(), "pool")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	mount := newMount(&runtime.Mount{Scope: "p", Path: root, Mode: runtime.ModeReadWrite})
	if err := mount.WriteFile("a.txt", []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mount.Rename("a.txt", "b.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := mount.Stat("a.txt"); !os.IsNotExist(err) {
		t.Fatalf("old name should be gone: %v", err)
	}
	if data, err := mount.ReadFile("b.txt"); err != nil || string(data) != "A" {
		t.Fatalf("new name = %q err=%v", data, err)
	}
}

// Load/A LoadFrom read the RuntimeSpec: SYY_RUNTIME_SPEC wins, otherwise the
// fixed container path is used; a missing spec is an identifiable error.
func TestLoadFromSpecFile(t *testing.T) {
	spec, admin, _ := testSpec(t)
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(t.TempDir(), "runtime.json")
	if err := os.WriteFile(specPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(runtime.EnvSpecPath, specPath)
	table, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if table.Source() != specPath {
		t.Fatalf("Source = %q", table.Source())
	}
	if local, err := table.LocalPathIn("admin1-main", "/ufi/pool-raw/syspool/Music/a.mp3"); err != nil || local != filepath.Join(admin, "a.mp3") {
		t.Fatalf("LocalPathIn = %q err=%v", local, err)
	}
	if _, err := LoadFrom(filepath.Join(t.TempDir(), "absent.json")); !errors.Is(err, ErrNoSpec) {
		t.Fatalf("LoadFrom(missing) = %v, want ErrNoSpec", err)
	}
	t.Setenv(runtime.EnvSpecPath, "")
	if _, err := Load(); !errors.Is(err, ErrNoSpec) {
		t.Fatalf("Load without spec = %v, want ErrNoSpec", err)
	}
}
