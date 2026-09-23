package appsdk

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appfs "github.com/siyouyun-open/siyouyun_sdk/v2/fs"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// writeMountTable writes a mount table and wires it to the current process's runtime contract.
func writeMountTable(t *testing.T, table protocol.MountTable) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, protocol.MountTableFileName)
	data, err := json.Marshal(table)
	if err != nil {
		t.Fatalf("marshal mount table: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write mount table: %v", err)
	}
	t.Setenv(protocol.EnvMountTable, path)
	return path
}

// multiSpaceRuntime builds a native instance serving the two spaces alice-main and bob-main.
//
// The pool directories must actually exist: Mount.Resolve validates that the real path of the
// "deepest existing ancestor" is inside the mount root, and a mount root that does not exist is
// treated as an escape (a deliberate safety design of the fs package).
func multiSpaceRuntime(t *testing.T) Runtime {
	t.Helper()
	root := t.TempDir()
	alicePool := filepath.Join(root, "alice", "syspool")
	aliceMedia := filepath.Join(root, "alice", "media")
	bobPool := filepath.Join(root, "bob", "syspool")
	appData := filepath.Join(root, "alice", ".app", "storage", "com.example.app")
	for _, dir := range []string{alicePool, aliceMedia, bobPool, appData} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("prepare pool directory %s: %v", dir, err)
		}
	}
	path := writeMountTable(t, protocol.MountTable{
		Version: protocol.MountTableVersion,
		AppData: &protocol.MountEntry{
			Space:     "alice-main",
			LocalPath: appData,
			Writable:  true,
			Kind:      protocol.MountKindAppData,
		},
		Pools: []protocol.MountEntry{
			{Name: "syspool", Space: "alice-main", LocalPath: alicePool, Writable: true},
			{Name: "media", Space: "alice-main", LocalPath: aliceMedia, Writable: true},
			{Name: "syspool", Space: "bob-main", LocalPath: bobPool, Writable: true},
		},
	})
	return Runtime{AppCode: "album", DataDir: t.TempDir(), MountTable: path}
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
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				if !errors.Is(err, ErrInvalidSpace) {
					t.Fatalf("wrong error type: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("space = %q, want %q", got, tc.want)
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

// TestAuthorizedSpaceResolves covers option A: spaces inside the authorized set can obtain a
// view, while everything outside the set is rejected.
func TestAuthorizedSpaceResolves(t *testing.T) {
	rt := multiSpaceRuntime(t)
	app := NewWithOptions(rt)

	view, err := app.Space("alice-main")
	if err != nil {
		t.Fatalf("alice-main should be authorized: %v", err)
	}
	if view.Name() != "alice-main" {
		t.Fatalf("Name = %q", view.Name())
	}
	wantDir := filepath.Join(rt.DataDir, "spaces", "alice-main")
	if view.AppDataDir() != wantDir {
		t.Fatalf("AppDataDir = %q, want %q", view.AppDataDir(), wantDir)
	}
	if pools := view.Pools(); len(pools) != 2 {
		t.Fatalf("alice-main pool count = %d, want 2", len(pools))
	}

	if _, err := app.Space("bob-main"); err != nil {
		t.Fatalf("bob-main should be authorized (same pool name in a different space): %v", err)
	}

	// Pools with the same name must resolve inside their own space — this is the easiest
	// place to get a native shared instance wrong.
	bobView, err := app.Space("bob-main")
	if err != nil {
		t.Fatalf("bob-main should be authorized: %v", err)
	}
	aliceMount, err := view.Pool("syspool")
	if err != nil {
		t.Fatalf("find syspool in alice space: %v", err)
	}
	bobMount, err := bobView.Pool("syspool")
	if err != nil {
		t.Fatalf("find syspool in bob space: %v", err)
	}
	if aliceMount.Root() == bobMount.Root() {
		t.Fatalf("pools with the same name in two spaces resolved to the same directory: %s", aliceMount.Root())
	}
	alicePath, err := view.LocalPath("syspool", "photos/a.jpg")
	if err != nil {
		t.Fatalf("resolve alice pool: %v", err)
	}
	bobPath, err := bobView.LocalPath("syspool", "photos/a.jpg")
	if err != nil {
		t.Fatalf("resolve bob pool: %v", err)
	}
	if alicePath != filepath.Join(aliceMount.Root(), "photos/a.jpg") {
		t.Fatalf("alice pool resolution = %q (root=%s)", alicePath, aliceMount.Root())
	}
	if bobPath != filepath.Join(bobMount.Root(), "photos/a.jpg") {
		t.Fatalf("bob pool resolution = %q (root=%s)", bobPath, bobMount.Root())
	}
	// Escapes are still blocked by the fs package; a space view cannot bypass it.
	if _, err := view.LocalPath("syspool", "../../../etc/passwd"); !errors.Is(err, appfs.ErrEscape) {
		t.Fatalf("escape path should be rejected, got %v", err)
	}
}

func TestUnauthorizedSpaceIsRejected(t *testing.T) {
	app := NewWithOptions(multiSpaceRuntime(t))

	_, err := app.Space("carol-main")
	if !errors.Is(err, ErrSpaceNotAuthorized) {
		t.Fatalf("unauthorized space should return ErrSpaceNotAuthorized, got %v", err)
	}
	// The error carries the authorized set, so troubleshooting needs no guesswork.
	if !strings.Contains(err.Error(), "alice-main") {
		t.Fatalf("error message should contain the authorized space list: %v", err)
	}
}

func TestInvalidSpaceNameIsRejectedBeforeMountLookup(t *testing.T) {
	app := NewWithOptions(multiSpaceRuntime(t))

	for _, name := range []string{"", "..", "../alice-main", "alice/main"} {
		_, err := app.Space(name)
		if !errors.Is(err, ErrInvalidSpace) {
			t.Fatalf("%q should return ErrInvalidSpace, got %v", name, err)
		}
	}
}

func TestSpaceRequiresDataDir(t *testing.T) {
	rt := multiSpaceRuntime(t)
	rt.DataDir = ""
	app := NewWithOptions(rt)

	if _, err := app.Space("alice-main"); !errors.Is(err, ErrNoSpaceDataDir) {
		t.Fatalf("missing DataDir should return ErrNoSpaceDataDir, got %v", err)
	}
}

// TestSpaceEnsureStaysInsideDataDir is a regression test for directory escape: even if an odd
// space name appears in the mount table (here a name with legal characters but a dot), the
// directory must stay inside DataDir.
func TestSpaceEnsureStaysInsideDataDir(t *testing.T) {
	rt := multiSpaceRuntime(t)
	writeMountTable(t, protocol.MountTable{
		Version: protocol.MountTableVersion,
		AppData: &protocol.MountEntry{Space: "a.b-main", LocalPath: "/pool/x", Writable: true, Kind: protocol.MountKindAppData},
	})
	app := NewWithOptions(rt)

	view, err := app.Space("a.b-main")
	if err != nil {
		t.Fatalf("a.b-main should be authorized: %v", err)
	}
	if err := view.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	info, err := os.Stat(view.AppDataDir())
	if err != nil || !info.IsDir() {
		t.Fatalf("Ensure did not create the directory: %v", err)
	}
	rel, err := filepath.Rel(rt.DataDir, view.AppDataDir())
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("space directory escaped DataDir: %q (rel=%q)", view.AppDataDir(), rel)
	}
}

func TestSpaceReportsMissingMountTable(t *testing.T) {
	t.Setenv(protocol.EnvMountTable, filepath.Join(t.TempDir(), "does-not-exist.json"))
	rt := Runtime{AppCode: "album", DataDir: t.TempDir()}
	app := NewWithOptions(rt)

	_, err := app.Space("alice-main")
	if !errors.Is(err, appfs.ErrNoMountTable) {
		t.Fatalf("missing mount table should return fs.ErrNoMountTable, got %v", err)
	}
}
