package appsdk

// Integration coverage for the additive multi-space RuntimeSpec contract shared
// with the runtime daemon (task-34 writer). Each fixture is the writer's real
// output shape; every absolute path is rewritten to a temp dir so the
// POSIX/escape checks run for real.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	appfs "github.com/siyouyun-open/siyouyun_sdk/v2/fs"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
	"github.com/siyouyun-open/siyouyun_sdk/v2/runtime"
)

func TestMultiSpaceSpecIntegration(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
	}{
		{name: "contract fixture", fixture: "runtime-spec-v1-multispace.json"},
		{name: "music writer sample", fixture: "runtime-spec-v1-music-multispace.sample.json"},
		{name: "telebook writer sample", fixture: "runtime-spec-v1-telebook-multispace.sample.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runMultiSpaceIntegration(t, filepath.Join("runtime", "testdata", tc.fixture))
		})
	}
}

// runMultiSpaceIntegration loads a multi-space fixture, rewrites its absolute
// paths to temp dirs, and exercises the full FromEnv -> Space -> mount path.
func runMultiSpaceIntegration(t *testing.T, fixturePath string) {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	base := t.TempDir()
	dirs := map[string]string{}
	for _, name := range []string{"install", "data", "run", "run/secrets", "cache"} {
		dir := filepath.Join(base, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		dirs[name] = dir
	}
	paths := spec["paths"].(map[string]any)
	paths["app"] = dirs["install"]
	paths["data"] = dirs["data"]
	paths["runtime"] = dirs["run"]
	paths["secrets"] = dirs["run/secrets"]
	paths["cache"] = dirs["cache"]
	bus := spec["bus"].(map[string]any)
	bus["socket"] = filepath.Join(dirs["run"], "bus.sock")

	// Every mount in these fixtures targets one logical scope and differs by
	// deployment space. Rewrite each target under the temp base.
	var scope, pool, root string
	wantSpaces := make([]string, 0)
	for _, rawSpace := range spec["spaces"].([]any) {
		wantSpaces = append(wantSpaces, rawSpace.(string))
	}
	mountRoots := map[string]string{}
	for _, rawMount := range spec["mounts"].([]any) {
		mount := rawMount.(map[string]any)
		space := mount["space"].(string)
		scope = mount["scope"].(string)
		pool = mount["pool"].(string)
		root = mount["root"].(string) // "" for a whole-space mount
		target := filepath.Join(base, "fuse", space, scope)
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		mount["path"] = target
		mountRoots[space] = target
	}
	if scope == "" || pool == "" || len(mountRoots) != len(wantSpaces) {
		t.Fatalf("fixture shape unexpected: scope=%q pool=%q root=%q mounts=%d spaces=%v",
			scope, pool, root, len(mountRoots), wantSpaces)
	}

	// Derive the read-only and writable spaces from the grants. A fixture may be
	// all read-only (whole-space/telebook variant), in which case rwSpace is "".
	roSpaces := make([]string, 0, len(wantSpaces))
	rwSpace := ""
	for _, rawGrant := range spec["grants"].([]any) {
		grant := rawGrant.(map[string]any)
		space := grant["space"].(string)
		writable := false
		for _, rawOp := range grant["ops"].([]any) {
			if rawOp.(string) == runtime.OpWrite {
				writable = true
			}
		}
		if writable {
			rwSpace = space
		} else {
			roSpaces = append(roSpaces, space)
		}
	}
	if len(roSpaces) == 0 {
		t.Fatalf("fixture needs at least one read-only space: grants=%v", spec["grants"])
	}

	rewritten, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(base, "runtime.json")
	if err := os.WriteFile(specPath, rewritten, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(runtime.EnvSpecPath, specPath)
	t.Setenv(protocol.EnvRuntime, "native")
	t.Setenv(protocol.EnvAppCode, "music")

	rt := FromEnv()
	if rt.SpecPath != specPath || rt.Spec == nil {
		t.Fatalf("spec not loaded: %#v", rt)
	}
	if got := rt.Spec.Spaces; len(got) != len(wantSpaces) || got[0] != wantSpaces[0] || got[1] != wantSpaces[1] {
		t.Fatalf("spaces = %v, want %v", got, wantSpaces)
	}
	if rt.DataDir != dirs["data"] || rt.InstallRoot != dirs["install"] {
		t.Fatalf("paths from spec = %q/%q", rt.DataDir, rt.InstallRoot)
	}

	app := NewWithOptions(rt)
	defer app.Close()

	// A scoped mount strips the scope root; a whole-space mount (root "") maps
	// the pool-relative path directly.
	rel := "2024/a.mp3"
	ufi := "/ufi/pool-raw/" + pool + "/" + rel
	if root != "" {
		ufi = "/ufi/pool-raw/" + pool + "/" + root + "/" + rel
	}
	for _, space := range wantSpaces {
		view, err := app.Space(space)
		if err != nil {
			t.Fatalf("Space(%s): %v", space, err)
		}
		local, err := view.LocalPathForUFI(ufi)
		if err != nil {
			t.Fatalf("LocalPathForUFI(%s): %v", space, err)
		}
		if want := filepath.Join(mountRoots[space], filepath.FromSlash(rel)); local != want {
			t.Fatalf("local(%s) = %q, want %q", space, local, want)
		}
	}

	// Read-only enforcement on a ro space.
	roView, err := app.Space(roSpaces[0])
	if err != nil {
		t.Fatalf("Space(%s): %v", roSpaces[0], err)
	}
	roMount, err := roView.Mount(scope)
	if err != nil {
		t.Fatalf("ro Mount: %v", err)
	}
	if err := roMount.WriteFile("x", []byte("x"), 0o644); !errors.Is(err, appfs.ErrReadOnly) {
		t.Fatalf("ro mount write = %v, want ErrReadOnly", err)
	}

	if rwSpace != "" {
		rwView, err := app.Space(rwSpace)
		if err != nil {
			t.Fatalf("Space(%s): %v", rwSpace, err)
		}
		rwLocal, err := rwView.LocalPathForUFI(ufi)
		if err != nil {
			t.Fatalf("rw LocalPathForUFI: %v", err)
		}
		rwMount, err := rwView.Mount(scope)
		if err != nil {
			t.Fatalf("rw Mount: %v", err)
		}
		if err := rwMount.MkdirAll(filepath.Dir(filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatalf("rw mkdir: %v", err)
		}
		if err := rwMount.WriteFile(filepath.FromSlash(rel), []byte("hello"), 0o644); err != nil {
			t.Fatalf("rw WriteFile: %v", err)
		}
		if data, err := os.ReadFile(rwLocal); err != nil || string(data) != "hello" {
			t.Fatalf("local file = %q err=%v", data, err)
		}
		// Grant expiry unit: expiresAt is Unix milliseconds (0 = never).
		if rwMount.ExpiresAt() > 0 {
			if rwMount.Expired(time.Now()) {
				t.Fatalf("rw mount should not be expired yet: expiresAt=%d", rwMount.ExpiresAt())
			}
			if !rwMount.Expired(time.UnixMilli(rwMount.ExpiresAt())) {
				t.Fatalf("rw mount must be expired at its ms timestamp: expiresAt=%d", rwMount.ExpiresAt())
			}
		}
	}

	// Unauthorised space is rejected with the stable sentinel.
	if _, err := app.Space("bob-main"); !errors.Is(err, ErrSpaceNotAuthorized) {
		t.Fatalf("Space(bob-main) = %v, want ErrSpaceNotAuthorized", err)
	}

	// App-private data comes from paths.data and is writable without a grant.
	dataDir, err := app.FS()
	if err != nil {
		t.Fatalf("FS: %v", err)
	}
	appData, err := dataDir.AppData()
	if err != nil {
		t.Fatalf("AppData: %v", err)
	}
	if err := appData.MkdirAll("posters", 0o755); err != nil {
		t.Fatalf("app data mkdir: %v", err)
	}
	if err := appData.WriteFile("posters/1.jpg", []byte("jpg"), 0o644); err != nil {
		t.Fatalf("app data write: %v", err)
	}
}
