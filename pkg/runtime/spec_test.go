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
	if got := spec.Bus(); got != wantBus {
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
	if ContainerSpecPath != "/run/syy/runtime.json" {
		t.Fatalf("ContainerSpecPath = %q, want /run/syy/runtime.json", ContainerSpecPath)
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
