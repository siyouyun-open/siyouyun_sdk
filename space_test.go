package appsdk

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appfs "github.com/siyouyun-open/siyouyun_sdk/v2/fs"
	"github.com/siyouyun-open/siyouyun_sdk/v2/runtime"
)

// multiSpaceSpec builds a native shared instance serving alice-main and bob-main:
//
//	alice-main: space.music -> alice-music (syspool, root Music), rw
//	alice-main: space       -> alice-space (data1, whole pool), ro
//	bob-main:   space.music -> bob-music  (syspool, root Music), ro
func multiSpaceSpec(t *testing.T) (*runtime.Spec, string) {
	t.Helper()
	root := t.TempDir()
	aliceMusic := filepath.Join(root, "alice", "music")
	aliceSpace := filepath.Join(root, "alice", "space")
	bobMusic := filepath.Join(root, "bob", "music")
	dataDir := filepath.Join(root, "appdata")
	for _, dir := range []string{aliceMusic, aliceSpace, bobMusic, dataDir} {
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
			App: filepath.Join(root, "app"), Data: dataDir,
			Runtime: filepath.Join(root, "run"), Secrets: filepath.Join(root, "run/secrets"),
			Cache: filepath.Join(root, "cache"),
		},
		Bus:    runtime.Bus{Socket: filepath.Join(root, "run/bus.sock")},
		Spaces: []string{"alice-main", "bob-main"},
		Mounts: []runtime.Mount{
			{Scope: "space.music", Space: "alice-main", Pool: "syspool", Root: "Music",
				Path: aliceMusic, Mode: runtime.ModeReadWrite, GrantID: "grant_alice_music"},
			{Scope: "space", Space: "alice-main", Pool: "data1", Root: "",
				Path: aliceSpace, Mode: runtime.ModeReadOnly, GrantID: "grant_alice_space"},
			{Scope: "space.music", Space: "bob-main", Pool: "syspool", Root: "Music",
				Path: bobMusic, Mode: runtime.ModeReadOnly, GrantID: "grant_bob_music"},
		},
		Grants: []runtime.Grant{
			{Scope: "space.music", Space: "alice-main", Ops: []string{runtime.OpRead, runtime.OpWrite}},
			{Scope: "space", Space: "alice-main", Ops: []string{runtime.OpRead}},
			{Scope: "space.music", Space: "bob-main", Ops: []string{runtime.OpRead}},
		},
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "runtime.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return spec, path
}

func TestSpaceFromHeaders(t *testing.T) {
	cases := []struct {
		name                   string
		username, group, space string
		want                   string
		wantErr                bool
	}{
		{name: "personal space", username: "alice", space: "main", want: "alice-main"},
		{name: "group space", username: "alice", group: "team", space: "main", want: "team-main"},
		{name: "group and user share the same name", username: "alice", group: "alice", space: "main", want: "alice-main"},
		{name: "with whitespace", username: " alice ", space: " main ", want: "alice-main"},
		{name: "private space", username: "alice", space: "private", want: "alice-private"},
		{name: "missing user", space: "main", wantErr: true},
		{name: "missing space", username: "alice", wantErr: true},
		{name: "all empty", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SpaceFromHeaders(tc.username, tc.group, tc.space)
			if tc.wantErr {
				if err == nil || !errors.Is(err, ErrInvalidSpace) {
					t.Fatalf("expected ErrInvalidSpace, got %q err=%v", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("space = %q err=%v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestValidSpaceName(t *testing.T) {
	valid := []string{"alice-main", "team-main", "a.b_c-d", "A1"}
	for _, s := range valid {
		if !ValidSpaceName(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	invalid := []string{"", ".", "..", "../etc", "alice/main", "alice main", "alice\nmain", strings.Repeat("x", 129)}
	for _, s := range invalid {
		if ValidSpaceName(s) {
			t.Errorf("%q should be rejected", s)
		}
	}
}

// Authorized spaces get a view with their private dir and the RuntimeSpec mounts.
func TestAuthorizedSpaceResolves(t *testing.T) {
	spec, specPath := multiSpaceSpec(t)
	app := NewWithOptions(Runtime{AppCode: "music", DataDir: spec.Paths.Data, SpecPath: specPath})
	defer app.Close()

	view, err := app.Space("alice-main")
	if err != nil {
		t.Fatalf("Space(alice-main): %v", err)
	}
	if view.Name() != "alice-main" {
		t.Fatalf("Name = %q", view.Name())
	}
	if want := filepath.Join(spec.Paths.Data, "spaces", "alice-main"); view.AppDataDir() != want {
		t.Fatalf("AppDataDir = %q, want %q", view.AppDataDir(), want)
	}
	if err := view.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if info, err := os.Stat(view.AppDataDir()); err != nil || !info.IsDir() {
		t.Fatalf("space data dir not created: %v", err)
	}
	if got := len(view.Mounts()); got != 2 {
		t.Fatalf("alice mounts = %d, want 2", got)
	}
	music, err := view.Mount("space.music")
	if err != nil || !music.Writable() || music.Pool() != "syspool" || music.ScopeRoot() != "Music" {
		t.Fatalf("Mount(space.music) = %+v err=%v", music, err)
	}
	// Pool lookup: syspool maps to exactly one scope mount in this space.
	pool, err := view.Pool("syspool")
	if err != nil || pool.Scope() != "space.music" {
		t.Fatalf("Pool(syspool) = %+v err=%v", pool, err)
	}
	if _, err := view.Pool("missing"); !errors.Is(err, appfs.ErrNoMount) {
		t.Fatalf("Pool(missing) = %v, want ErrNoMount", err)
	}

	// UFI -> local path inside the authorised space.
	local, err := view.LocalPathForUFI("/ufi/pool-raw/syspool/Music/2024/a.mp3")
	if err != nil {
		t.Fatalf("LocalPathForUFI: %v", err)
	}
	wantSuffix := filepath.Join("2024", "a.mp3")
	if !strings.HasSuffix(local, wantSuffix) {
		t.Fatalf("LocalPathForUFI = %q, want suffix %q", local, wantSuffix)
	}
	// Pool-relative resolution goes through the scope root as well.
	local, err = view.LocalPath("syspool", "Music/2024/a.mp3")
	if err != nil || !strings.HasSuffix(local, wantSuffix) {
		t.Fatalf("LocalPath = %q err=%v", local, err)
	}
	// Whole-space mount resolves pool-relative paths directly.
	if _, err := view.LocalPathForUFI("/ufi/pool-raw/data1/Documents/a.pdf"); err != nil {
		t.Fatalf("whole-space UFI: %v", err)
	}
	// A UFI from another pool in this space is not resolvable.
	if _, err := view.LocalPathForUFI("/ufi/pool-raw/other/Music/a.mp3"); !errors.Is(err, appfs.ErrNoMount) {
		t.Fatalf("unknown pool = %v, want ErrNoMount", err)
	}

	// bob-main is a read-only space with its own mount.
	bob, err := app.Space("bob-main")
	if err != nil {
		t.Fatalf("Space(bob-main): %v", err)
	}
	bobMusic, err := bob.Mount("space.music")
	if err != nil || bobMusic.Writable() {
		t.Fatalf("bob mount = %+v err=%v", bobMusic, err)
	}
	if _, err := bob.LocalPathForUFI("/ufi/pool-raw/syspool/Music/a.mp3"); err != nil {
		t.Fatalf("bob UFI: %v", err)
	}
	// The two spaces resolve the same UFI to different local paths.
	aliceLocal, _ := view.LocalPathForUFI("/ufi/pool-raw/syspool/Music/a.mp3")
	bobLocal, _ := bob.LocalPathForUFI("/ufi/pool-raw/syspool/Music/a.mp3")
	if aliceLocal == bobLocal {
		t.Fatalf("cross-space UFI resolution must not collapse: %q", aliceLocal)
	}
}

// Spaces outside the spec's authorized set are rejected before any IO.
func TestUnauthorizedSpaceIsRejected(t *testing.T) {
	spec, specPath := multiSpaceSpec(t)
	app := NewWithOptions(Runtime{AppCode: "music", DataDir: spec.Paths.Data, SpecPath: specPath})
	defer app.Close()

	_, err := app.Space("carol-main")
	if !errors.Is(err, ErrSpaceNotAuthorized) {
		t.Fatalf("Space(carol-main) = %v, want ErrSpaceNotAuthorized", err)
	}
	if _, err := app.Space(""); !errors.Is(err, ErrInvalidSpace) {
		t.Fatalf("Space(empty) = %v, want ErrInvalidSpace", err)
	}
	if _, err := app.Space("../etc/passwd"); !errors.Is(err, ErrInvalidSpace) {
		t.Fatalf("Space(traversal) = %v, want ErrInvalidSpace", err)
	}
}

// A view needs paths.data: without it per-space data has no home.
func TestSpaceRequiresDataDir(t *testing.T) {
	_, specPath := multiSpaceSpec(t)
	app := NewWithOptions(Runtime{AppCode: "music", SpecPath: specPath})
	defer app.Close()
	if _, err := app.Space("alice-main"); !errors.Is(err, ErrNoSpaceDataDir) {
		t.Fatalf("Space without paths.data = %v, want ErrNoSpaceDataDir", err)
	}
}

// When the spec is unavailable the error is propagated instead of being hidden.
func TestSpaceReportsMissingSpec(t *testing.T) {
	app := NewWithOptions(Runtime{AppCode: "music", DataDir: t.TempDir()})
	defer app.Close()
	if _, err := app.Space("alice-main"); !errors.Is(err, appfs.ErrNoSpec) {
		t.Fatalf("Space without spec = %v, want ErrNoSpec", err)
	}
}
