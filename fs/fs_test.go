package fs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

func writeTable(t *testing.T, dir string, table protocol.MountTable) string {
	t.Helper()
	path := filepath.Join(dir, protocol.MountTableFileName)
	data, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
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

func TestLoadAndResolveSingleSpace(t *testing.T) {
	base := t.TempDir()
	poolRoot := filepath.Join(base, "pool")
	appData := filepath.Join(base, "appdata")
	for _, dir := range []string{poolRoot, appData} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := writeTable(t, base, protocol.MountTable{
		Version: protocol.MountTableVersion,
		AppData: &protocol.MountEntry{LocalPath: appData, Writable: true, Kind: protocol.MountKindAppData},
		Pools: []protocol.MountEntry{
			{Name: "syspool", Space: "alice-main", LocalPath: poolRoot, Writable: true},
		},
	})

	table, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got := table.Spaces(); len(got) != 1 || got[0] != "alice-main" {
		t.Fatalf("Spaces = %#v", got)
	}
	app, err := table.AppData()
	if err != nil || app.Root() != appData || !app.Writable() {
		t.Fatalf("AppData = %v err=%v", app, err)
	}

	// Single space: the name is unique, so no space needs to be specified.
	pool, err := table.Pool("syspool")
	if err != nil || pool.Root() != poolRoot {
		t.Fatalf("Pool = %v err=%v", pool, err)
	}
	if _, err := table.Pool("missing"); !errors.Is(err, ErrNoMount) {
		t.Fatalf("missing pool must fail, got %v", err)
	}

	// UFI → local path.
	local, err := table.LocalPath("/ufi/pool-raw/syspool/photos/a.jpg")
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	if want := filepath.Join(poolRoot, "photos/a.jpg"); local != want {
		t.Fatalf("LocalPath = %q, want %q", local, want)
	}
	// UGN → space → UFI.
	local, err = table.LocalPathForUGN("alice", "alice", "main", "/ufi/pool-raw/syspool/x")
	if err != nil {
		t.Fatalf("LocalPathForUGN: %v", err)
	}
	if want := filepath.Join(poolRoot, "x"); local != want {
		t.Fatalf("LocalPathForUGN = %q, want %q", local, want)
	}
}

// In a multi-space shared instance (native), a pool with a duplicate name must be given an explicit space; silently taking the first one would write data to the wrong place.
func TestAmbiguousPoolRequiresSpace(t *testing.T) {
	base := t.TempDir()
	poolA := filepath.Join(base, "a")
	poolB := filepath.Join(base, "b")
	for _, dir := range []string{poolA, poolB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := writeTable(t, base, protocol.MountTable{
		Version: protocol.MountTableVersion,
		Pools: []protocol.MountEntry{
			{Name: "syspool", Space: "alice-main", LocalPath: poolA, Writable: true},
			{Name: "syspool", Space: "bob-main", LocalPath: poolB, Writable: true},
		},
	})
	table, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := table.Pool("syspool"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("ambiguous Pool must fail, got %v", err)
	}
	pool, err := table.PoolIn("bob-main", "syspool")
	if err != nil || pool.Root() != poolB {
		t.Fatalf("PoolIn = %v err=%v", pool, err)
	}
	if _, err := table.LocalPath("/ufi/pool-raw/syspool/x"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("ambiguous LocalPath must fail, got %v", err)
	}
	if _, err := table.LocalPathForUGN("bob", "bob", "main", "/ufi/pool-raw/syspool/x"); err != nil {
		t.Fatalf("LocalPathForUGN(bob) = %v", err)
	}
	if _, err := table.LocalPathForUGN("carol", "carol", "main", "/ufi/pool-raw/syspool/x"); !errors.Is(err, ErrNoMount) {
		t.Fatalf("unauthorized space must fail, got %v", err)
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

	path := writeTable(t, base, protocol.MountTable{
		Version: protocol.MountTableVersion,
		Pools:   []protocol.MountEntry{{Name: "p", LocalPath: root, Writable: true}},
	})
	table, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mount, _ := table.Pool("p")

	for _, rel := range []string{"..", "../outside/secret", "a/../../outside", "/../../etc/passwd", "link/secret"} {
		if _, err := mount.Resolve(rel); !errors.Is(err, ErrEscape) {
			t.Fatalf("Resolve(%q) must be rejected, got %v", rel, err)
		}
	}
	// Both ordinary relative paths and forms with a leading "/" are accepted.
	for _, rel := range []string{"", ".", "a/b.txt", "/a/b.txt", "a/../b.txt"} {
		if _, err := mount.Resolve(rel); err != nil {
			t.Fatalf("Resolve(%q) = %v", rel, err)
		}
	}
	if err := mount.RemoveAll("link"); err != nil {
		t.Fatalf("RemoveAll(link) = %v", err)
	}
}

func TestMountPOSIXOps(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "pool")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	mount, ok := newMount(protocol.MountEntry{Name: "p", LocalPath: root, Writable: true})
	if !ok {
		t.Fatal("mount entry rejected")
	}

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
	if got := mount.UFI("photos/2026/a.txt"); got != "/ufi/pool-raw/p/photos/2026/a.txt" {
		t.Fatalf("UFI = %q", got)
	}
	if err := mount.Remove("photos/2026/a.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestReadOnlyMountRejectsWrites(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "pool")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mount, ok := newMount(protocol.MountEntry{Name: "p", LocalPath: root, Writable: false})
	if !ok {
		t.Fatal("mount entry rejected")
	}

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
	src, ok := newMount(protocol.MountEntry{Name: "src", LocalPath: srcRoot, Writable: true})
	if !ok {
		t.Fatal("src mount rejected")
	}
	dst, ok := newMount(protocol.MountEntry{Name: "dst", LocalPath: dstRoot, Writable: true})
	if !ok {
		t.Fatal("dst mount rejected")
	}

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
	base := t.TempDir()
	root := filepath.Join(base, "pool")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	mount, ok := newMount(protocol.MountEntry{Name: "p", LocalPath: root, Writable: true})
	if !ok {
		t.Fatal("mount entry rejected")
	}
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

func TestLoadPrefersEnvAndFallsBackToContainerPath(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "pool")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeTable(t, base, protocol.MountTable{
		Version: protocol.MountTableVersion,
		Pools:   []protocol.MountEntry{{Name: "p", LocalPath: root, Writable: true}},
	})

	t.Setenv(protocol.EnvMountTable, path)
	table, err := Load()
	if err != nil {
		t.Fatalf("Load from env: %v", err)
	}
	if len(table.Pools()) != 1 {
		t.Fatalf("pools = %#v", table.Pools())
	}

	// When the environment variable is missing, fall back to the container path; it does not exist on the test machine, so an identifiable error must be returned.
	t.Setenv(protocol.EnvMountTable, "")
	if _, err := Load(); !errors.Is(err, ErrNoMountTable) {
		t.Fatalf("Load without table = %v, want ErrNoMountTable", err)
	}
}

func TestLoadRejectsBadVersionAndEmptyTable(t *testing.T) {
	base := t.TempDir()
	path := writeTable(t, base, protocol.MountTable{Version: 99})
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "unsupported mount table version") {
		t.Fatalf("bad version must fail, got %v", err)
	}

	// An empty table can be loaded, but it has no usable mounts.
	empty := writeTable(t, base, protocol.MountTable{Version: protocol.MountTableVersion})
	table, err := LoadFile(empty)
	if err != nil {
		t.Fatalf("empty table: %v", err)
	}
	if len(table.Pools()) != 0 {
		t.Fatalf("unexpected pools: %#v", table.Pools())
	}
	if _, err := table.AppData(); !errors.Is(err, ErrNoMount) {
		t.Fatalf("AppData on empty table = %v", err)
	}
}

// native narrowing: a host process has no mount namespace, so a pool is accessible as a whole
// directory; narrowing can only be enforced by the SDK according to the scope declarations in
// the mount table.
func TestTableScopeEnforcement(t *testing.T) {
	dir := t.TempDir()
	poolRoot := filepath.Join(dir, "syspool")
	if err := os.MkdirAll(filepath.Join(poolRoot, "Music", "2024"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := protocol.MountTable{
		Version: protocol.MountTableVersion,
		Pools: []protocol.MountEntry{
			{Name: "syspool", Space: "alice-main", LocalPath: poolRoot, Writable: true, Kind: protocol.MountKindPool},
		},
	}

	music := "/ufi/pool-raw/syspool/Music/2024/a.mp3"
	parent := "/ufi/pool-raw/syspool/Music/a.mp3"
	other := "/ufi/pool-raw/syspool/Documents/a.pdf"

	// 1) No scope declaration: no narrowing (behavior unchanged for old packages/old daemons).
	path := writeTable(t, dir, base)
	table, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !table.InScope(other) {
		t.Fatal("must not narrow when there is no declaration")
	}
	if _, err := table.LocalPathIn("alice-main", other); err != nil {
		t.Fatalf("should resolve when there is no declaration: %v", err)
	}
	if len(table.Scopes()) != 0 {
		t.Fatalf("scopes = %v", table.Scopes())
	}

	// 2) Narrow to space.music/2024.
	scoped := base
	scoped.Scopes = []string{"space.music/2024"}
	table, err = LoadFile(writeTable(t, dir, scoped))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := table.LocalPathIn("alice-main", music); err != nil {
		t.Fatalf("a path inside the scope should resolve: %v", err)
	}
	if _, err := table.LocalPathIn("alice-main", parent); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("a parent directory must be blocked by the scope, got %v", err)
	}
	if _, err := table.LocalPathIn("alice-main", other); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("another business directory must be blocked by the scope, got %v", err)
	}
	// Space axis: a pool that does not belong to this space is also not let through (it is first
	// blocked by "this space has no such pool"; scope checking is a second independent gate, and
	// both count as a rejection).
	if _, err := table.LocalPathIn("bob-main", music); !errors.Is(err, ErrOutOfScope) && !errors.Is(err, ErrNoMount) {
		t.Fatalf("another space must be blocked, got %v", err)
	}
	// The reconciliation path uses the same set of checks.
	if !table.InScope(music) || table.InScope(parent) || table.InScope(other) {
		t.Fatal("InScope and LocalPath must agree")
	}
	if !table.InScopeIn("alice-main", music) || table.InScopeIn("bob-main", music) {
		t.Fatal("InScopeIn's space-axis decision is wrong")
	}

	// 3) all.photos: the space axis lets everything through, filtering by path only.
	device := base
	device.Pools = append(device.Pools, protocol.MountEntry{
		Name: "bob_syspool", Space: "bob-main", LocalPath: poolRoot, Writable: true, Kind: protocol.MountKindPool,
	})
	device.Scopes = []string{"all.photos"}
	table, err = LoadFile(writeTable(t, dir, device))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := table.LocalPathIn("bob-main", "/ufi/pool-raw/bob_syspool/Photos/a.jpg"); err != nil {
		t.Fatalf("a device-level scope should let another space through: %v", err)
	}
	if _, err := table.LocalPathIn("bob-main", "/ufi/pool-raw/bob_syspool/Music/a.mp3"); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("a device-level scope is still constrained by path, got %v", err)
	}

	// 4) Invalid declarations are ignored; when all of them are invalid it degrades to "no narrowing" (better usable than letting the app read no files at all because of one bad config entry).
	broken := base
	broken.Scopes = []string{"device.all", "space.unknownkey"}
	table, err = LoadFile(writeTable(t, dir, broken))
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Scopes()) != 0 {
		t.Fatalf("invalid values must not enter the table: %v", table.Scopes())
	}
	if _, err := table.LocalPathIn("alice-main", other); err != nil {
		t.Fatalf("must not narrow when there is no valid declaration: %v", err)
	}
}

// 授权空间列表（protocol.MountTable.Spaces）必须并入 Spaces()。
//
// 背景：应用侧空间闸门是 `App.Space(space) → Table.HasSpace(space) → Table.Spaces()`，
// 而 AppData 只有一个指针。共享 native 实例的数据空间是 default、又要服务 admin-main
// 用户时，用户空间只能靠 WebNamespaces 表达 —— 没有这个字段时它进不了授权集合，
// 每次调用都被应用拒成
//
//	space not authorized: "admin-main" (instance authorized spaces: default)
//
// 有 storage.user.file 池的应用靠 Pools[].Space 捎带才有该空间；没有池的应用直接不可用。
func TestTableSpacesIncludesExplicitAuthorizationList(t *testing.T) {
	base := t.TempDir()
	appData := filepath.Join(base, "appdata")
	if err := os.MkdirAll(appData, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeTable(t, base, protocol.MountTable{
		Version: protocol.MountTableVersion,
		AppData: &protocol.MountEntry{Space: "default", LocalPath: appData, Writable: true, Kind: protocol.MountKindAppData},
		// 没有池：这正是 telebook 的形态，授权空间只能来自 Spaces。
		Spaces: []string{"admin-main", "default", ""},
	})

	table, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	got := table.Spaces()
	want := []string{"admin-main", "default"}
	if len(got) != len(want) {
		t.Fatalf("Spaces = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Spaces = %#v, want %#v", got, want)
		}
	}
	// 它就是空间闸门的判据。
	if !table.HasSpace("admin-main") {
		t.Fatal("HasSpace(admin-main) 必须为真，否则应用会 403")
	}
	if table.HasSpace("bob-main") {
		t.Fatal("未授权的空间不能放行")
	}
	// 授权列表不产生挂载点：没有池就没有 Pool。
	if _, err := table.Pool("syspool"); err == nil {
		t.Fatal("Spaces 只是授权列表，不该造出存储池")
	}
}

// 老运行时（不写 spaces 字段）行为必须完全不变。
func TestTableSpacesUnchangedWithoutExplicitList(t *testing.T) {
	base := t.TempDir()
	poolRoot := filepath.Join(base, "pool")
	if err := os.MkdirAll(poolRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeTable(t, base, protocol.MountTable{
		Version: protocol.MountTableVersion,
		Pools:   []protocol.MountEntry{{Name: "syspool", Space: "alice-main", LocalPath: poolRoot, Writable: true}},
	})
	table, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got := table.Spaces(); len(got) != 1 || got[0] != "alice-main" {
		t.Fatalf("Spaces = %#v, want [alice-main]", got)
	}
}
