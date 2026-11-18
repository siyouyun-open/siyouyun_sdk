# SDK v2: RuntimeSpec v1 migration checklist (music / movie / telebook)

- Status: SDK reader/data plane implemented and covered by tests; the minictl writer (task-34) produces the spec against the same contract
- Applies to: `github.com/siyouyun-open/siyouyun_sdk/v2` (this worktree, `feat/runtime-spec`)
- Conclusion: **the legacy `mounts.json` / `MINI_APP_MOUNT_TABLE` / `NATIVE_STORAGE_POOL_*` data plane has been removed, with no compatibility and no fallback**; an app updated per this document can build locally with a replace

## 1. Old vs. new changes

| Item | Old (v2.2 / mounts.json) | New (RuntimeSpec v1) |
| --- | --- | --- |
| native data-plane file | `MINI_APP_MOUNT_TABLE` → `<state>/<appcode>/mounts.json` | `SYY_RUNTIME_SPEC` → `<root>/<appcode>/run/runtime.json` |
| container data-plane file | `/etc/miniruntime/resources/mounts.json` | fixed `/lacx/run/runtime.json` (aligned with minictl's `/lacx` unification; **not** the old SDK main's `/run/syy`) |
| storage pool paths | `NATIVE_STORAGE_POOL_*` environment variables | not injected; only inside spec `mounts[]` |
| authorised spaces | `MountTable.Spaces` (AppData/Pools/explicit list) | spec top-level `spaces[]` + `mounts[].space` |
| pool/scope root | `MountEntry.Name/Space/LocalPath` | `mounts[].pool` + `mounts[].root` (pool-relative root of the scope) |
| ro/rw | `MountEntry.Writable` | double-checked against `mounts[].mode` (`ro`/`rw`) and `grants[].ops` |
| app private directory | `MountEntry{Kind: app_data}` | `paths.data` (not part of mounts) |
| SDK entry points | `fs.Load/LoadFile`, `Table.Pool*/SpacePools/InScope` | `fs.Load/LoadFrom/New(spec)`, `Table.Mount*/Mounts/SpaceMounts/LocalPath*` |

**Unchanged** (app-facing): `appsdk.New/NewWithOptions/Runtime`, `application.Space(...)`,
`SpaceView.Ensure/AppDataDir/Name`, `SpaceView.LocalPath/LocalPathForUFI`,
`SpaceFromHeaders`, `ValidSpaceName`, `ErrInvalidSpace/ErrSpaceNotAuthorized`,
`capability.StartFileEventListener`, and the bus/KV/event capabilities.

## 2. RuntimeSpec v1 contract (including additive extensions)

Fixed fields (v1, `additionalProperties=false`, the SDK parses with `DisallowUnknownFields`):
`version`, `app{id,code,version,instanceId,namespace}`,
`paths{app,data,runtime,secrets,cache}`, `bus{socket,http?}`, `mounts[]`, `grants[]`.

additive extensions (new in this migration; all optional, no version bump):

```jsonc
{
  "spaces": ["admin1-main", "admin1-private"],      // deployment spaces served by this instance (writer outputs them in alphabetical order)
  "mounts": [
    {
      "scope": "music",                              // core scopeKey (manifest `space.music` → `music`; whole space `space` → `space`)

      "space": "admin1-main",                        // deployment space
      "pool": "syspool",                             // pool name inside the UFI
      "root": "Music",                               // pool-relative root of the scope; "" or "/" for the whole space
      "path": "/run/lacx/grantfs/<instance>/grants/space.music.admin1-main", // mount target the application actually opens
      "mode": "ro",                                  // ro | rw
      "grantId": "grant_...",                        // must not be empty
      "expiresAt": 0                                 // Unix milliseconds (core/daemon unit), 0 = no expiry
    }
  ],
  "grants": [
    {"scope": "space.music", "space": "admin1-main", "ops": ["read"], "expiresAt": 0}
  ]
}
```

Validation (the SDK rejects the whole spec):
- `version == 1`; path fields absolute and clean; `mounts[].path != "/"`; `expiresAt >= 0`; **expiresAt is in Unix milliseconds** (core grants are passed through as-is, and the daemon expiry timer also uses ms);
- `spaces[]`: non-empty, no leading/trailing whitespace, `^[A-Za-z0-9][A-Za-z0-9._-]*$`, no duplicates;
- `mounts[]/grants[]` uniqueness: **`(space, scope)` is unique** (one entry per space for the same scope); `mounts[].path` is globally unique;
- `mounts[].space/pool/root` rules: setting `pool`/`root` ⇒ `space` must also be set; setting `root` ⇒ `pool` must also be set; `root` is a clean pool-relative path (leading/trailing `/` allowed, NUL/`.`/`..`/empty segments forbidden);
- a mount and a grant must match (first by `(space,scope)`, then falling back to a space-less grant); `ro` needs `read`, `rw` needs `write`, otherwise `fs.ErrNoGrant` / `fs.ErrGrantDenied`.

Reference fixtures (all in the SDK repo under `runtime/testdata/`):
- `runtime-spec-v1-multispace.json`: contract/semantics fixture (scope shaped like `space.music`);
- `runtime-spec-v1-music-multispace.sample.json`: **real output of the minictl writer** (music; scope=`music`, root=`Music`, admin1-main ro / admin1-private rw, `expiresAt` in milliseconds);
- `runtime-spec-v1-telebook-multispace.sample.json`: **real output of the minictl writer** (telebook; scope=`space`, root=`""` whole space, both spaces ro, pool=`syspool`).

Writer output essentials (reconciliation conclusion of task-34):
- `mounts[].scope` is the **core scopeKey** (manifest `space.music` → `music`; `space` → `space`), not the manifest literal; the SDK only validates the pattern and does not make business decisions on the scope literal, so an app must not assume the scope name equals the manifest key (use `SpaceView.LocalPathForUFI` or `view.Mounts()` when it needs to locate a directory);
- `mounts[].path` looks like `/run/lacx/grantfs/<instanceId>/grants/<space>/<scope>` (unique per `(space,scope)`; the SDK does not parse the structure and opens it as-is);
- `spaces[]` is output in alphabetical order; every mount/grant entry carries `space` (except for legacy instances without space information, which keep the old shape and omit these fields);
- single-space native also writes `space/pool/root` + `spaces:[install space]`;
- `expiresAt` is **Unix milliseconds** (core grants are passed through as-is; the schema documents this).

## 3. SDK API mapping (app side)

### 3.1 Runtime / entry points

```go
rt := app.Runtime          // still normalised by FromEnv
rt.AppID / rt.AppCode / rt.InstanceID / rt.Namespace / rt.Port / rt.Endpoint
rt.InstanceID / rt.RuntimeKind / rt.IsNative() / rt.IsContainer() / rt.DefaultScope()
rt.InstallRoot   // = spec.paths.app
rt.DataDir       // = spec.paths.data (app private directory)
rt.BusSocket / rt.BusHTTP / rt.BusWS / rt.CallerToken
rt.Spec          // *runtime.Spec (nil during local development / without a spec)
rt.SpecPath      // spec file location (used by ReloadFS)
```
- Removed: `rt.MountTable`, `rt.StoragePools`, `rt.FindStoragePool(...)`.
- identity/bus/port/token/PG DSN are still injected as environment variables by the daemon; **the data plane reads from the spec only**.

### 3.2 Data plane (fs)

```go
table, err := app.FS()                 // parse the spec; no spec → fs.ErrNoSpec
table.ReloadFS()                       // re-read after the spec is regenerated

mount, err := table.Mount("space.music")             // unique scope
mount, err = table.MountIn("alice-main", "space.music") // multi-space must pass the space
data, err := mount.ReadFile("2024/a.mp3")            // relative to the mount root (a leading / is allowed)
f, err := mount.OpenFile("x", os.O_RDWR|os.O_CREATE, 0o644) // write on a ro mount → fs.ErrReadOnly

table.Mounts() / table.Scopes() / table.Spaces() / table.HasSpace(space)
table.AppData()                                      // paths.data (*fs.Mount, readable and writable)
table.LocalPath(ufi) / table.LocalPathIn(space, ufi)
table.LocalPathForUGN(username, group, namespace, ufi)
table.PoolResolve(space, pool, rel)
```
Removed: the env/fixed-container-path semantics of `fs.Load`, `Table.Pool/PoolIn/AppData(old)/SpacePools/InScope/InScopeIn`,
`fs.ErrNoMountTable/ErrOutOfScope`; added/kept errors: `ErrNoSpec/ErrNoMount/ErrAmbiguous/ErrBadUFI/ErrEscape/ErrReadOnly/ErrNoGrant/ErrGrantDenied/ErrNoAppData`.

### 3.3 Space view (`App.Space`)

```go
view, err := app.Space(space)   // validation chain: ValidSpaceName → spec.spaces → paths.data
errors.Is(err, appsdk.ErrInvalidSpace)        // 400
errors.Is(err, appsdk.ErrSpaceNotAuthorized)  // 403 (space ∉ spec.spaces)
errors.Is(err, appsdk.ErrNoSpaceDataDir)      // 500 (no paths.data)
errors.Is(err, appfs.ErrNoSpec)               // 503 (daemon has not written the spec)

view.Name() / view.AppDataDir() / view.Ensure()
view.Mounts() / view.Mount(scope)
view.Pools()  // grant-backed mounts of this space
view.Pool(name) // usable when the pool is covered by exactly one scope; multiple scopes → fs.ErrAmbiguous
view.LocalPath(pool, rel)
view.LocalPathForUFI(ufi)
```
UFI resolution rule: `/ufi/pool-raw/<pool>/<rel>` → match by `pool` among the mounts (of the given space),
trim the pool-internal prefix with `root` (`root==""` uses rel directly; requires `rel==root` or the prefix `root+"/"`),
then join it onto `mount.path` and run escape/symlink checks; 0 matches → `ErrNoMount`, multiple matches → `ErrAmbiguous`.

### 3.4 Where app private files live

- Write new files to `app.DataDir()` (one per app) or `view.AppDataDir()` (one per space, created by `Ensure()`).
- Legacy in-pool UFIs (such as `/ufi/pool-raw/syspool/.siyouyun/appdata/<app>/...`) are only resolvable when some mount covers that directory;
  normally there is none → **a one-off migration or on-demand regeneration is required** (e.g. re-scrape covers from the media), do not assume the old paths remain usable.

## 4. Minimal migration checklist for the three apps

### music (task-30)
- [x] `go.mod` replace to this worktree; drop the `contrib/watchdirs` dependency
- [x] events: `application.Events().Subscribe` → `capability.StartFileEventListener(ctx, app.Bus(), opts, handler)`; `Dirs: []string{"/Music"}`, `MediaTypes: audio`, `Actions: add/delete/rename`, `TransportBus`; delete the `/watch/dirs` API
- [x] manifest: drop `{value: space}` from `scopes`, keep `space.music` + `app`; bump `version` (per branch rules)
- [ ] private files (cover cache): migrate from the in-pool `/.siyouyun/appdata/music` to `app.DataDir()`/`view.AppDataDir()` (migrate or regenerate old files)
- note the scope the writer outputs is the core scopeKey (`music`), not the manifest's `space.music`
- [ ] unit tests: `internal/testspec` using a real spec (JSON + `SYY_RUNTIME_SPEC` + `FromEnv`) is ready; mind the `runtime.Spec.Bus` field name (if constructing via struct)
- removed old usage: the appFilesPool check of `view.Pool("syspool")` (multiple scopes on the same pool → `ErrAmbiguous`)

### movie (task-31)
- [x] `go.mod` replace; delete `cmd/v2/watch_dirs*`
- [x] events switch to `capability.StartFileEventListener` (`Dirs: /Movies`, a logical directory, not a scope key)
- [ ] manifest: drop `{value: space}`, keep `space.movies` + `app`; bump the version; `contract.open_with` as needed (access: posix / entry "/")
- [ ] posters/stills: write new files to `app.DataDir()`/`view.AppDataDir()`; do a one-off migration or on-demand re-scrape of the old `syspool/.siyouyun/appdata/movie` UFIs, and do not fail the whole page when reads fail
- [ ] multi-pool historical UFIs: only pools covered by the current scope's authorised root can be resolved; other old records need migration/re-scanning; the scope the writer outputs is the core scopeKey (e.g. `movies`), not the manifest's `space.movies`
- [ ] test injection: `NewWithOptions(Runtime{AppCode, DataDir, SpecPath})` or `SYY_RUNTIME_SPEC`+env + `FromEnv`; the mount directory must really exist (escape validation resolves the deepest existing ancestor)
- keep movie's existing fix: scraped images tmp+fsync+rename

### telebook (task-32)
- [x] `go.mod` replace (comment included: remove the replace before release)
- [ ] `application.Space(space)` / `SpaceFromHeaders` / `ErrSpaceNotAuthorized` stay unchanged; contact photos via `LocalPathForUFI` depend on the minictl-written `space/pool/root` mounts
- [ ] manifest: narrow to the directories actually accessed (drop `{value: space}` if only app-private/DB access is needed; keep the concrete `space.<dir>` and document it if photos need a file directory); on the writer side telebook's scope is `space` (core scopeKey) with an empty root (whole space)
- [ ] `SpaceView.Ensure()` can be dropped (PG metadata does not depend on it); if kept, use it for private files only
- [ ] `MINI_APP_PG_DSN` is still injected and the PG schema logic is unchanged
- [ ] test injection as above; note the old `Runtime{MountTable: ...}` construction is no longer available

## 5. Local replace build (how 104 is verified)

```go
// <app>/go.mod
replace github.com/siyouyun-open/siyouyun_sdk/v2 => /Users/weikeliu/workspace/jzxl/wt-sdk-v2-runtime
```
```bash
cd /Users/weikeliu/workspace/jzxl/wt-faas-apps/app/<music|movie|telebook>
go build ./... && go test ./... -count=1
```
- Once the replace takes effect no network pull of an SDK tag is needed; the published v2.x lines in `go.sum` can be kept; before release remove the replace and switch to the SDK tag (v2.4.0 recommended, pending the Lead's release window).
- Current status (2026-11-18): with their own worktree changes all three apps (music / movie / telebook) **pass `go build ./...`** (replaced to this worktree).
- The 103 real data plane needs the minictl writer (task-34) to write the spec with the §2 fields; until then unit tests construct the spec as described in §6.

## 6. How unit tests inject the spec

```go
// Option A: give Runtime a temporary spec directly (recommended for unit tests)
specPath := filepath.Join(t.TempDir(), "runtime.json")
os.WriteFile(specPath, specJSON, 0o600)
app := appsdk.NewWithOptions(appsdk.Runtime{
    AppCode: "music", DataDir: spec.Paths.Data, SpecPath: specPath,
})

// Option B: same as production (SYY_RUNTIME_SPEC + environment identity)
t.Setenv(runtime.EnvSpecPath, specPath)
t.Setenv(protocol.EnvRuntime, "native")   // plus the minimal set of APPCODE/MINI_APP_ID/... required
app := appsdk.New()

// Option C: use the Runtime from `appsdk.FromEnv()` directly for construction
rt := appsdk.FromEnv()
table, err := appfs.LoadFrom(rt.SpecPath)
```
Notes:
- The spec's `paths.*` and `mounts[].path` must be **existing** temporary directories (escape validation resolves the deepest existing ancestor), otherwise mount resolution rejects them;
- A fixture can copy `runtime/testdata/runtime-spec-v1-multispace.json` and replace the absolute paths with `t.TempDir()`;
- Mind the `runtime.Spec` field names: `Bus` (not `BusConfig`), `Mounts []Mount`, `Grants []Grant`; the `Mode`/`Op` enum constants live in the `runtime` package.

## 7. Known limitations / TODO

1. **The minictl writer (task-34) is implemented**: per §2 it writes `spaces` + `mounts{space,pool,root}` + `grants[].space` and queries/mounts per space; the two real samples are checked in as golden fixtures for the integration test (`TestMultiSpaceSpecIntegration` passes all three cases).
2. **Protocol sync**: the SDK has removed `protocol/mounts.go`; the host-side `pkg/protocol` `contract_test.go`/`testdata/contract_vectors.json` still carry the `mount_table` section, so `scripts/check-protocol-sync.sh` reports a diff and the host side must remove it too. In addition the SDK `protocol/env.go` still keeps a few env constants used for local SDK development (`MINI_APP_INSTALL_ROOT/DATA_DIR/...`) that the host side does not have; this is a pre-existing difference from the repository split, and a byte-for-byte sync requires both sides to agree first (see the delivery report for details).
3. **SDK tag**: v2.4.0 recommended, to be released in a window decided by the Lead; this worktree does not commit/push/tag.
4. **Container form**: the path is `/lacx/run/runtime.json`; the container writer may optionally add the same `space/pool/root` fields (V3 multi-space does not need them yet).
