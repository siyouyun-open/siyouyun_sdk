package runtime

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const validSpecJSON = `{
  "version": 1,
  "app": {
    "id": "com.lacx.videoplayer",
    "code": "videoplayer",
    "version": "0.1.0",
    "instanceId": "inst-1",
    "namespace": "admin1-main"
  },
  "paths": {
    "app": "/app",
    "data": "/data",
    "runtime": "/run/syy",
    "secrets": "/run/syy/secrets",
    "cache": "/cache"
  },
  "bus": {
    "socket": "/run/syy/bus.sock",
    "http": "http://127.0.0.1:41000"
  },
  "mounts": [
    {"scope": "photos", "path": "/mnt/grants/photos", "mode": "ro", "grantId": "grant_photos", "expiresAt": 0},
    {"scope": "downloads", "path": "/mnt/grants/downloads", "mode": "rw", "grantId": "grant_downloads"}
  ],
  "grants": [
    {"scope": "photos", "ops": ["read"], "expiresAt": 0},
    {"scope": "downloads", "ops": ["read", "write"]}
  ]
}`

func writeSpecFile(t *testing.T, content string) string {
	t.Helper()
	specPath := filepath.Join(t.TempDir(), "runtime.json")
	if err := os.WriteFile(specPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return specPath
}

func validObject(t *testing.T) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal([]byte(validSpecJSON), &obj); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return obj
}

func writeObjectFile(t *testing.T, obj any) string {
	t.Helper()
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("encode spec: %v", err)
	}
	return writeSpecFile(t, string(data))
}

func parseObject(t *testing.T, obj any) (*Spec, error) {
	t.Helper()
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("encode spec: %v", err)
	}
	return Parse(data)
}

func objectSection(t *testing.T, obj map[string]any, key string) map[string]any {
	t.Helper()
	section, ok := obj[key].(map[string]any)
	if !ok {
		t.Fatalf("fixture %q is not an object", key)
	}
	return section
}

func arraySection(t *testing.T, obj map[string]any, key string) []any {
	t.Helper()
	section, ok := obj[key].([]any)
	if !ok {
		t.Fatalf("fixture %q is not an array", key)
	}
	return section
}

func firstObjectIn(t *testing.T, obj map[string]any, key string) map[string]any {
	t.Helper()
	items := arraySection(t, obj, key)
	if len(items) == 0 {
		t.Fatalf("fixture %q is empty", key)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("fixture %q[0] is not an object", key)
	}
	return item
}

func TestLoadFromValidSpec(t *testing.T) {
	spec, err := LoadFrom(writeSpecFile(t, validSpecJSON))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if spec.Version != 1 {
		t.Fatalf("version = %d, want 1", spec.Version)
	}
	wantApp := App{
		ID:         "com.lacx.videoplayer",
		Code:       "videoplayer",
		Version:    "0.1.0",
		InstanceID: "inst-1",
		Namespace:  "admin1-main",
	}
	if spec.App != wantApp {
		t.Fatalf("app = %+v, want %+v", spec.App, wantApp)
	}

	if got, want := spec.AppDir(), "/app"; got != want {
		t.Fatalf("AppDir = %q, want %q", got, want)
	}
	if got, want := spec.DataDir(), "/data"; got != want {
		t.Fatalf("DataDir = %q, want %q", got, want)
	}
	if got, want := spec.CacheDir(), "/cache"; got != want {
		t.Fatalf("CacheDir = %q, want %q", got, want)
	}
	if got, want := spec.RuntimeDir(), "/run/syy"; got != want {
		t.Fatalf("RuntimeDir = %q, want %q", got, want)
	}
	if got, want := spec.SecretsDir(), "/run/syy/secrets"; got != want {
		t.Fatalf("SecretsDir = %q, want %q", got, want)
	}
	wantBus := Bus{Socket: "/run/syy/bus.sock", HTTP: "http://127.0.0.1:41000"}
	if got := spec.Bus; got != wantBus {
		t.Fatalf("Bus = %+v, want %+v", got, wantBus)
	}

	mount, err := spec.Mount("photos")
	if err != nil {
		t.Fatalf("Mount(photos): %v", err)
	}
	if mount.Path != "/mnt/grants/photos" {
		t.Fatalf("photos path = %q", mount.Path)
	}
	if mount.Mode != ModeReadOnly || mount.Writable() {
		t.Fatalf("photos mode = %q, want ro", mount.Mode)
	}
	if mount.GrantID != "grant_photos" {
		t.Fatalf("photos grantId = %q", mount.GrantID)
	}

	mountPath, mode, err := spec.MountPath("downloads")
	if err != nil {
		t.Fatalf("MountPath(downloads): %v", err)
	}
	if mountPath != "/mnt/grants/downloads" || mode != ModeReadWrite {
		t.Fatalf("downloads = (%q, %q), want (/mnt/grants/downloads, rw)", mountPath, mode)
	}
	if got, want := spec.Scopes(), []string{"photos", "downloads"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Scopes = %v, want %v", got, want)
	}

	if !spec.Grants[0].Allows(OpRead) || spec.Grants[0].Allows(OpWrite) {
		t.Fatalf("photos grant ops = %v, want read only", spec.Grants[0].Ops)
	}
	if !spec.Grants[1].Allows(OpWrite) {
		t.Fatalf("downloads grant ops = %v, want write", spec.Grants[1].Ops)
	}
}

func TestParseAllowsEmptyMountsAndGrants(t *testing.T) {
	obj := validObject(t)
	obj["mounts"] = []any{}
	obj["grants"] = []any{}

	spec, err := parseObject(t, obj)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(spec.Mounts) != 0 || len(spec.Scopes()) != 0 {
		t.Fatalf("mounts = %v, want none", spec.Mounts)
	}
}

func TestLoadFromMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.json")
	if _, err := LoadFrom(missing); !errors.Is(err, ErrNoSpec) {
		t.Fatalf("LoadFrom(missing) error = %v, want ErrNoSpec", err)
	}
	if _, err := LoadFrom("   "); !errors.Is(err, ErrNoSpec) {
		t.Fatalf("LoadFrom(blank) error = %v, want ErrNoSpec", err)
	}
}

func TestParseRejectsBadJSON(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"truncated", `{"version": 1`},
		{"array root", `[]`},
		{"unknown field", `{"version": 1, "unexpected": true}`},
		{"wrong version type", `{"version": "1"}`},
		{"trailing data", `{"version": 1} {"version": 1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse([]byte(tc.body)); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("Parse(%q) error = %v, want ErrInvalidSpec", tc.body, err)
			}
		})
	}
}

func TestParseEnforcesVersionOne(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing version", func(obj map[string]any) { delete(obj, "version") }},
		{"version zero", func(obj map[string]any) { obj["version"] = 0 }},
		{"version two", func(obj map[string]any) { obj["version"] = 2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := validObject(t)
			tc.mutate(obj)
			if _, err := parseObject(t, obj); !errors.Is(err, ErrUnsupportedVersion) {
				t.Fatalf("error = %v, want ErrUnsupportedVersion", err)
			}
		})
	}
}

func TestParseRejectsDuplicateMountScope(t *testing.T) {
	obj := validObject(t)
	mounts := arraySection(t, obj, "mounts")
	obj["mounts"] = append(mounts, map[string]any{
		"scope":   "photos",
		"path":    "/mnt/grants/photos-again",
		"mode":    "ro",
		"grantId": "grant_other",
	})

	_, err := parseObject(t, obj)
	if !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("error = %v, want ErrInvalidSpec", err)
	}
	if !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("error = %v, want duplicate scope detail", err)
	}
}

func TestParseRejectsDuplicateMountPath(t *testing.T) {
	obj := validObject(t)
	mounts := arraySection(t, obj, "mounts")
	obj["mounts"] = append(mounts, map[string]any{
		"scope":   "music",
		"path":    "/mnt/grants/photos",
		"mode":    "ro",
		"grantId": "grant_music",
	})

	if _, err := parseObject(t, obj); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("error = %v, want ErrInvalidSpec", err)
	}
}

func TestParseRejectsDuplicateGrant(t *testing.T) {
	obj := validObject(t)
	grants := arraySection(t, obj, "grants")
	obj["grants"] = append(grants, map[string]any{
		"scope": "photos",
		"ops":   []any{"read"},
	})

	if _, err := parseObject(t, obj); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("error = %v, want ErrInvalidSpec", err)
	}
}

func TestParseRejectsDuplicateGrantOp(t *testing.T) {
	obj := validObject(t)
	firstObjectIn(t, obj, "grants")["ops"] = []any{"read", "read"}

	if _, err := parseObject(t, obj); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("error = %v, want ErrInvalidSpec", err)
	}
}

func TestParseRequiresAbsolutePaths(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"relative paths.data", func(obj map[string]any) {
			objectSection(t, obj, "paths")["data"] = "data"
		}},
		{"empty paths.app", func(obj map[string]any) {
			objectSection(t, obj, "paths")["app"] = ""
		}},
		{"relative paths.secrets", func(obj map[string]any) {
			objectSection(t, obj, "paths")["secrets"] = "run/syy/secrets"
		}},
		{"relative bus.socket", func(obj map[string]any) {
			objectSection(t, obj, "bus")["socket"] = "bus.sock"
		}},
		{"mount is host root", func(obj map[string]any) {
			firstObjectIn(t, obj, "mounts")["path"] = "/"
		}},
		{"relative mount path", func(obj map[string]any) {
			firstObjectIn(t, obj, "mounts")["path"] = "mnt/grants/photos"
		}},
		{"trailing slash path", func(obj map[string]any) {
			objectSection(t, obj, "paths")["data"] = "/data/"
		}},
		{"unclean path", func(obj map[string]any) {
			objectSection(t, obj, "paths")["runtime"] = "/run//syy"
		}},
		{"dot-dot path", func(obj map[string]any) {
			objectSection(t, obj, "paths")["cache"] = "/cache/../tmp"
		}},
		{"nul in path", func(obj map[string]any) {
			objectSection(t, obj, "paths")["secrets"] = "/run/syy/secrets\x00"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := validObject(t)
			tc.mutate(obj)
			if _, err := parseObject(t, obj); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("error = %v, want ErrInvalidSpec", err)
			}
		})
	}
}

func TestParseValidatesBusHTTP(t *testing.T) {
	cases := []struct {
		name string
		http string
	}{
		{"missing scheme", "127.0.0.1:41000"},
		{"bad scheme", "ftp://127.0.0.1:41000"},
		{"missing host", "http://"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := validObject(t)
			objectSection(t, obj, "bus")["http"] = tc.http
			if _, err := parseObject(t, obj); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("http %q: error = %v, want ErrInvalidSpec", tc.http, err)
			}
		})
	}

	// An empty HTTP endpoint is valid for a UDS-only deployment.
	obj := validObject(t)
	delete(objectSection(t, obj, "bus"), "http")
	if _, err := parseObject(t, obj); err != nil {
		t.Fatalf("UDS-only spec: %v", err)
	}
}

func TestParseRequiresValidMountMode(t *testing.T) {
	for _, mode := range []string{"", "write", "RO", "rwx"} {
		t.Run(mode, func(t *testing.T) {
			obj := validObject(t)
			firstObjectIn(t, obj, "mounts")["mode"] = mode
			if _, err := parseObject(t, obj); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("mode %q: error = %v, want ErrInvalidSpec", mode, err)
			}
		})
	}
}

func TestParseRequiresGrantFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"empty mount grantId", func(obj map[string]any) {
			firstObjectIn(t, obj, "mounts")["grantId"] = ""
		}},
		{"blank mount grantId", func(obj map[string]any) {
			firstObjectIn(t, obj, "mounts")["grantId"] = "   "
		}},
		{"empty grant ops", func(obj map[string]any) {
			firstObjectIn(t, obj, "grants")["ops"] = []any{}
		}},
		{"unknown grant op", func(obj map[string]any) {
			firstObjectIn(t, obj, "grants")["ops"] = []any{"execute"}
		}},
		{"empty grant scope", func(obj map[string]any) {
			firstObjectIn(t, obj, "grants")["scope"] = ""
		}},
		{"negative mount expiry", func(obj map[string]any) {
			firstObjectIn(t, obj, "mounts")["expiresAt"] = -1
		}},
		{"negative grant expiry", func(obj map[string]any) {
			firstObjectIn(t, obj, "grants")["expiresAt"] = -1
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := validObject(t)
			tc.mutate(obj)
			if _, err := parseObject(t, obj); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("error = %v, want ErrInvalidSpec", err)
			}
		})
	}
}

func TestMountUnknownScope(t *testing.T) {
	spec, err := LoadFrom(writeSpecFile(t, validSpecJSON))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if _, err := spec.Mount("music"); !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("Mount(music) error = %v, want ErrUnknownScope", err)
	}
	if _, _, err := spec.MountPath("music"); !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("MountPath(music) error = %v, want ErrUnknownScope", err)
	}
	if _, err := spec.Mount("Photos"); !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("Mount(Photos) error = %v, want ErrUnknownScope", err)
	}
}

func TestLoadUsesNativeEnv(t *testing.T) {
	specPath := writeSpecFile(t, validSpecJSON)
	t.Setenv(EnvSpecPath, specPath)

	spec, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := spec.DataDir(), "/data"; got != want {
		t.Fatalf("DataDir = %q, want %q", got, want)
	}

	// The native bootstrap variable wins over the container location.
	if _, err := load(specPath, filepath.Join(t.TempDir(), "absent.json")); err != nil {
		t.Fatalf("load with env path: %v", err)
	}
}

func TestLoadFallsBackToContainerPath(t *testing.T) {
	if ContainerSpecPath != "/lacx/run/runtime.json" {
		t.Fatalf("ContainerSpecPath = %q, want /lacx/run/runtime.json", ContainerSpecPath)
	}

	specPath := writeSpecFile(t, validSpecJSON)
	for _, envPath := range []string{"", "   "} {
		spec, err := load(envPath, specPath)
		if err != nil {
			t.Fatalf("load(%q, containerPath): %v", envPath, err)
		}
		if got, want := spec.AppDir(), "/app"; got != want {
			t.Fatalf("AppDir = %q, want %q", got, want)
		}
	}

	missing := filepath.Join(t.TempDir(), "absent.json")
	if _, err := load("", missing); !errors.Is(err, ErrNoSpec) {
		t.Fatalf("load(blank, missing) error = %v, want ErrNoSpec", err)
	}
}

func TestLoadIgnoresLegacyEnv(t *testing.T) {
	for _, key := range []string{
		"MINI_APP_DATA_DIR",
		"MINI_APP_INSTALL_ROOT",
		"MINI_APP_STATIC_ROOT",
		"NATIVE_STORAGE_POOL_MAIN_POOL",
		"NATIVE_STORAGE_POOL_APP_DATA",
	} {
		t.Setenv(key, "/tmp/legacy-"+strings.ToLower(key))
	}
	t.Setenv(EnvSpecPath, "")

	specPath := writeSpecFile(t, validSpecJSON)
	spec, err := load(os.Getenv(EnvSpecPath), specPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got, want := spec.DataDir(), "/data"; got != want {
		t.Fatalf("DataDir = %q, want %q (legacy env must be ignored)", got, want)
	}
	if got, want := spec.AppDir(), "/app"; got != want {
		t.Fatalf("AppDir = %q, want %q (legacy env must be ignored)", got, want)
	}
}

// multiSpaceSpecJSON is the additive v1 extension used by native shared
// instances: top-level spaces + per-mount space/pool/root + per-grant space.
const multiSpaceSpecJSON = `{
  "version": 1,
  "app": {
    "id": "music",
    "code": "music",
    "version": "2.4.1",
    "instanceId": "inst-music",
    "namespace": "default"
  },
  "paths": {
    "app": "/app",
    "data": "/appdata",
    "runtime": "/run/lacx",
    "secrets": "/run/lacx/secrets",
    "cache": "/cache"
  },
  "bus": {"socket": "/run/lacx/bus.sock"},
  "spaces": ["admin1-main", "alice-main"],
  "mounts": [
    {"scope": "space.music", "space": "admin1-main", "pool": "syspool", "root": "Music",
     "path": "/fuse/admin1/music", "mode": "ro", "grantId": "grant_a"},
    {"scope": "space.music", "space": "alice-main", "pool": "syspool", "root": "/Music/",
     "path": "/fuse/alice/music", "mode": "rw", "grantId": "grant_b"}
  ],
  "grants": [
    {"scope": "space.music", "space": "admin1-main", "ops": ["read"]},
    {"scope": "space.music", "space": "alice-main", "ops": ["read", "write"]}
  ]
}`

func TestParseKeepsMultiSpaceFields(t *testing.T) {
	spec, err := Parse([]byte(multiSpaceSpecJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := spec.Spaces, []string{"admin1-main", "alice-main"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Spaces = %v, want %v", got, want)
	}
	mount, err := spec.MountIn("alice-main", "space.music")
	if err != nil {
		t.Fatalf("MountIn: %v", err)
	}
	if mount.Pool != "syspool" || mount.Root != "/Music/" || mount.Space != "alice-main" {
		t.Fatalf("mount = %+v", mount)
	}
	if mount.Writable() != true {
		t.Fatalf("alice mount should be writable: %+v", mount)
	}
	// The same scope in several spaces is ambiguous without a space.
	if _, err := spec.Mount("space.music"); !errors.Is(err, ErrAmbiguousScope) {
		t.Fatalf("Mount(scope) error = %v, want ErrAmbiguousScope", err)
	}
	if _, err := spec.Grant("space.music"); !errors.Is(err, ErrAmbiguousScope) {
		t.Fatalf("Grant(scope) error = %v, want ErrAmbiguousScope", err)
	}
	grant, err := spec.GrantIn("admin1-main", "space.music")
	if err != nil || grant.Allows(OpWrite) {
		t.Fatalf("admin grant = %+v err=%v", grant, err)
	}
	if got, want := spec.Scopes(), []string{"space.music"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Scopes = %v, want %v", got, want)
	}
}

// A space-less grant is the fallback for a space-qualified mount.
func TestGrantInFallsBackToSpaceLessGrant(t *testing.T) {
	obj := validObject(t)
	obj["spaces"] = []any{"admin1-main"}
	mount := firstObjectIn(t, obj, "mounts")
	mount["space"] = "admin1-main"
	mount["pool"] = "syspool"
	mount["root"] = "Photos"
	grant := firstObjectIn(t, obj, "grants")
	delete(grant, "space")

	spec, err := parseObject(t, obj)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := spec.GrantIn("admin1-main", "photos")
	if err != nil || !got.Allows(OpRead) {
		t.Fatalf("GrantIn fallback = %+v err=%v", got, err)
	}
	// A space-less grant applies to every space (older single-space writers).
	if got, err := spec.GrantIn("other-space", "photos"); err != nil || !got.Allows(OpRead) {
		t.Fatalf("GrantIn(other space) = %+v err=%v", got, err)
	}
}

func TestParseRejectsInvalidMultiSpaceFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"duplicate space", func(obj map[string]any) {
			obj["spaces"] = []any{"admin1-main", "admin1-main"}
		}},
		{"empty space", func(obj map[string]any) {
			obj["spaces"] = []any{""}
		}},
		{"pool without space", func(obj map[string]any) {
			firstObjectIn(t, obj, "mounts")["pool"] = "syspool"
		}},
		{"root without pool", func(obj map[string]any) {
			firstObjectIn(t, obj, "mounts")["root"] = "Music"
		}},
		{"root with dot-dot", func(obj map[string]any) {
			mount := firstObjectIn(t, obj, "mounts")
			mount["space"] = "admin1-main"
			mount["pool"] = "syspool"
			mount["root"] = "Music/../Documents"
		}},
		{"duplicate scope in same space", func(obj map[string]any) {
			mounts := arraySection(t, obj, "mounts")
			first := mounts[0].(map[string]any)
			first["space"] = "admin1-main"
			first["pool"] = "syspool"
			obj["mounts"] = append(mounts, map[string]any{
				"scope": first["scope"], "space": "admin1-main", "pool": "syspool",
				"path": "/mnt/grants/other", "mode": "ro", "grantId": "grant_other",
			})
		}},
		{"duplicate grant in same space", func(obj map[string]any) {
			grants := arraySection(t, obj, "grants")
			first := grants[0].(map[string]any)
			first["space"] = "admin1-main"
			obj["grants"] = append(grants, map[string]any{
				"scope": first["scope"], "space": "admin1-main", "ops": []any{"read"},
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := validObject(t)
			tc.mutate(obj)
			if _, err := parseObject(t, obj); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("error = %v, want ErrInvalidSpec", err)
			}
		})
	}
}

// TestParseMultiSpaceFixture freezes the additive multi-space contract shared
// with the runtime daemon (task-34 writer): top-level spaces + per-mount
// space/pool/root + per-grant space.
func TestParseMultiSpaceFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "runtime-spec-v1-multispace.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	spec, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse fixture: %v", err)
	}
	if got, want := spec.Spaces, []string{"admin1-main", "alice-main"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Spaces = %v, want %v", got, want)
	}
	if len(spec.Mounts) != 2 || len(spec.Grants) != 2 {
		t.Fatalf("mounts/grants = %d/%d, want 2/2", len(spec.Mounts), len(spec.Grants))
	}
	alice, err := spec.MountIn("alice-main", "space.music")
	if err != nil || !alice.Writable() || alice.Pool != "syspool" || alice.Root != "Music" {
		t.Fatalf("alice mount = %+v err=%v", alice, err)
	}
	admin, err := spec.MountIn("admin1-main", "space.music")
	if err != nil || admin.Writable() {
		t.Fatalf("admin mount = %+v err=%v", admin, err)
	}
}

// TestParseMusicSampleGolden freezes the real writer output for the music app:
// scope = core scopeKey ("music"), spaces sorted, per-space mounts with
// space/pool/root, expiresAt in Unix milliseconds.
func TestParseMusicSampleGolden(t *testing.T) {
	spec := loadSampleSpec(t, "runtime-spec-v1-music-multispace.sample.json")
	if got, want := spec.Spaces, []string{"admin1-main", "admin1-private"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Spaces = %v, want %v", got, want)
	}
	main, err := spec.MountIn("admin1-main", "music")
	if err != nil {
		t.Fatalf("MountIn(admin1-main, music): %v", err)
	}
	private, err := spec.MountIn("admin1-private", "music")
	if err != nil {
		t.Fatalf("MountIn(admin1-private, music): %v", err)
	}
	if main.Mode != ModeReadOnly || main.Pool != "syspool" || main.Root != "Music" ||
		main.GrantID != "grant_AAAAAAAAAAAAAAAA" ||
		main.Path != "/run/lacx/grantfs/native-music-0123456789abcdef/grants/admin1-main/music" {
		t.Fatalf("admin1-main mount = %+v", main)
	}
	if private.Mode != ModeReadWrite || private.ExpiresAt != 1893456000000 ||
		private.Path != "/run/lacx/grantfs/native-music-0123456789abcdef/grants/admin1-private/music" {
		t.Fatalf("admin1-private mount = %+v", private)
	}
	// Same scope in two spaces is ambiguous without the space.
	if _, err := spec.Mount("music"); !errors.Is(err, ErrAmbiguousScope) {
		t.Fatalf("Mount(music) = %v, want ErrAmbiguousScope", err)
	}
	// Per-space ops: admin1-main read-only, admin1-private read+write.
	mainGrant, err := spec.GrantIn("admin1-main", "music")
	if err != nil || mainGrant.Allows(OpWrite) {
		t.Fatalf("admin1-main grant = %+v err=%v", mainGrant, err)
	}
	privateGrant, err := spec.GrantIn("admin1-private", "music")
	if err != nil || !privateGrant.Allows(OpWrite) || privateGrant.ExpiresAt != 1893456000000 {
		t.Fatalf("admin1-private grant = %+v err=%v", privateGrant, err)
	}
}

// TestParseTelebookSampleGolden freezes the whole-space variant: scope "space",
// root "", both spaces read-only, same pool "syspool".
func TestParseTelebookSampleGolden(t *testing.T) {
	spec := loadSampleSpec(t, "runtime-spec-v1-telebook-multispace.sample.json")
	if got, want := spec.Spaces, []string{"admin1-main", "admin1-private"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Spaces = %v, want %v", got, want)
	}
	for _, space := range spec.Spaces {
		mount, err := spec.MountIn(space, "space")
		if err != nil {
			t.Fatalf("MountIn(%s, space): %v", space, err)
		}
		if mount.Mode != ModeReadOnly || mount.Pool != "syspool" || mount.Root != "" ||
			!strings.Contains(mount.Path, "/grants/"+space+"/space") {
			t.Fatalf("%s mount = %+v", space, mount)
		}
		grant, err := spec.GrantIn(space, "space")
		if err != nil || grant.Allows(OpWrite) {
			t.Fatalf("%s grant = %+v err=%v", space, grant, err)
		}
	}
}

// loadSampleSpec parses a writer sample fixture.
func loadSampleSpec(t *testing.T, name string) *Spec {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read sample %s: %v", name, err)
	}
	spec, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse sample %s: %v", name, err)
	}
	return spec
}
