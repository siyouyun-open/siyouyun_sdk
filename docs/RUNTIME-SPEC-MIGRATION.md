# SDK v2：RuntimeSpec v1 迁移清单（music / movie / telebook）

- 状态：SDK reader/data plane 已实现并通过测试；minictl writer（task-34）按同一契约产出 spec
- 适用：`github.com/siyouyun-open/siyouyun_sdk/v2`（本 worktree `feat/runtime-spec`）
- 结论：**旧 `mounts.json` / `MINI_APP_MOUNT_TABLE` / `NATIVE_STORAGE_POOL_*` 数据面已删除，无兼容、无回退**；app 按本文改完即可在本地 replace 构建

## 1. 变更新旧对照

| 项 | 旧（v2.2 / mounts.json） | 新（RuntimeSpec v1） |
| --- | --- | --- |
| native 数据面文件 | `MINI_APP_MOUNT_TABLE` → `<state>/<appcode>/mounts.json` | `SYY_RUNTIME_SPEC` → `<root>/<appcode>/run/runtime.json` |
| 容器数据面文件 | `/etc/miniruntime/resources/mounts.json` | 固定 `/lacx/run/runtime.json`（对齐 minictl 的 `/lacx` 统一；**不是** old SDK main 的 `/run/syy`） |
| 存储池路径 | `NATIVE_STORAGE_POOL_*` 环境变量 | 不注入；只在 spec `mounts[]` 里 |
| 授权空间 | `MountTable.Spaces`（AppData/Pools/显式列表） | spec 顶层 `spaces[]` + `mounts[].space` |
| 池/scope 根 | `MountEntry.Name/Space/LocalPath` | `mounts[].pool` + `mounts[].root`（scope 在池内的相对根）|
| ro/rw | `MountEntry.Writable` | `mounts[].mode`（`ro`/`rw`）与 `grants[].ops` 双重校验 |
| app 私有目录 | `MountEntry{Kind: app_data}` | `paths.data`（不入 mounts） |
| SDK 入口 | `fs.Load/LoadFile`、`Table.Pool*/SpacePools/InScope` | `fs.Load/LoadFrom/New(spec)`、`Table.Mount*/Mounts/SpaceMounts/LocalPath*` |

**保持不变**（app-facing）：`appsdk.New/NewWithOptions/Runtime`、`application.Space(...)`、
`SpaceView.Ensure/AppDataDir/Name`、`SpaceView.LocalPath/LocalPathForUFI`、
`SpaceFromHeaders`、`ValidSpaceName`、`ErrInvalidSpace/ErrSpaceNotAuthorized`、
`capability.StartFileEventListener`、总线/KV/事件等能力。

## 2. RuntimeSpec v1 契约（含 additive 扩展）

固定字段（v1，`additionalProperties=false`，SDK 用 `DisallowUnknownFields` 解析）：
`version`、`app{id,code,version,instanceId,namespace}`、
`paths{app,data,runtime,secrets,cache}`、`bus{socket,http?}`、`mounts[]`、`grants[]`。

additive 扩展（本迁移新增；均为可选，不 bump version）：

```jsonc
{
  "spaces": ["admin1-main", "admin1-private"],      // 实例被授权服务的部署空间（writer 按字母序输出）
  "mounts": [
    {
      "scope": "music",                              // core scopeKey（manifest `space.music` → `music`；整空间 `space` → `space`）

      "space": "admin1-main",                        // 部署空间
      "pool": "syspool",                             // UFI 里的池名
      "root": "Music",                               // scope 在池内的相对根；整空间为 "" 或 "/"
      "path": "/run/lacx/grantfs/<instance>/grants/space.music.admin1-main", // 应用实际打开的挂载目标
      "mode": "ro",                                  // ro | rw
      "grantId": "grant_...",                        // 非空
      "expiresAt": 0                                 // Unix 毫秒（core/daemon 单位），0=不过期
    }
  ],
  "grants": [
    {"scope": "space.music", "space": "admin1-main", "ops": ["read"], "expiresAt": 0}
  ]
}
```

校验（SDK 会拒绝整个 spec）：
- `version == 1`；路径字段绝对且 clean；`mounts[].path != "/"`；`expiresAt >= 0`；**expiresAt 单位是 Unix 毫秒**（core grant 原样透传，daemon 过期定时器也用 ms）；
- `spaces[]`：非空、无首尾空白、`^[A-Za-z0-9][A-Za-z0-9._-]*$`、不重复；
- `mounts[]/grants[]` 唯一性：**`(space, scope)` 唯一**（同一 scope 每个空间一条）；`mounts[].path` 全局唯一；
- `mounts[].space/pool/root` 规则：设 `pool`/`root` ⇒ 必须设 `space`；设 `root` ⇒ 必须设 `pool`；`root` 为 clean 池内相对路径（允许首尾 `/`，禁止 NUL/`.`/`..`/空段）；
- mount 与 grant 必须匹配（先 `(space,scope)`，再 space-less grant 回退），`ro` 需 `read`、`rw` 需 `write`，否则 `fs.ErrNoGrant` / `fs.ErrGrantDenied`。

参考 fixture（都在 SDK 仓 `runtime/testdata/`）：
- `runtime-spec-v1-multispace.json`：契约/语义 fixture（scope 形如 `space.music`）；
- `runtime-spec-v1-music-multispace.sample.json`：**minictl writer 真实产出**（music；scope=`music`、root=`Music`、admin1-main ro / admin1-private rw、`expiresAt` 毫秒）；
- `runtime-spec-v1-telebook-multispace.sample.json`：**minictl writer 真实产出**（telebook；scope=`space`、root=`""` 整空间、两空间均 ro、pool=`syspool`）。

writer 实际产出要点（task-34 对账结论）：
- `mounts[].scope` 是 **core scopeKey**（manifest `space.music` → `music`；`space` → `space`），不是 manifest 字面量；SDK 只按 pattern 校验、不按 scope 字面做业务判断，app 不要假设 scope 名等于 manifest key（需要定位目录时用 `SpaceView.LocalPathForUFI` 或 `view.Mounts()`）；
- `mounts[].path` 形如 `/run/lacx/grantfs/<instanceId>/grants/<space>/<scope>`（每 `(space,scope)` 唯一；SDK 不解析结构，原样打开）；
- `spaces[]` 按字母序输出；mount/grants 每条都带 `space`（legacy 无空间信息的老实例除外，保持旧形状不写这些字段）；
- 单空间 native 也会写 `space/pool/root` + `spaces:[安装空间]`；
- `expiresAt` 是 **Unix 毫秒**（core grant 原样透传，schema 已标注）。

## 3. SDK API 对照（app 侧）

### 3.1 Runtime / 入口

```go
rt := app.Runtime          // 仍由 FromEnv 归一化
rt.AppID / rt.AppCode / rt.InstanceID / rt.Namespace / rt.Port / rt.Endpoint
rt.InstanceID / rt.RuntimeKind / rt.IsNative() / rt.IsContainer() / rt.DefaultScope()
rt.InstallRoot   // = spec.paths.app
rt.DataDir       // = spec.paths.data（app 私有目录）
rt.BusSocket / rt.BusHTTP / rt.BusWS / rt.CallerToken
rt.Spec          // *runtime.Spec（本地开发/无 spec 时为 nil）
rt.SpecPath      // spec 文件位置（ReloadFS 用）
```
- 已删除：`rt.MountTable`、`rt.StoragePools`、`rt.FindStoragePool(...)`。
- identity/bus/port/token/PG DSN 仍由 daemon 注入环境变量；**数据面只从 spec 读**。

### 3.2 数据面（fs）

```go
table, err := app.FS()                 // 解析 spec；无 spec → fs.ErrNoSpec
table.ReloadFS()                       // spec 重新生成后重读

mount, err := table.Mount("space.music")             // 唯一 scope
mount, err = table.MountIn("alice-main", "space.music") // 多空间必须带 space
data, err := mount.ReadFile("2024/a.mp3")            // 相对 mount root（可带前导 /）
f, err := mount.OpenFile("x", os.O_RDWR|os.O_CREATE, 0o644) // ro mount 写 → fs.ErrReadOnly

table.Mounts() / table.Scopes() / table.Spaces() / table.HasSpace(space)
table.AppData()                                      // paths.data（*fs.Mount，可读可写）
table.LocalPath(ufi) / table.LocalPathIn(space, ufi)
table.LocalPathForUGN(username, group, namespace, ufi)
table.PoolResolve(space, pool, rel)
```
已删除：`fs.Load` 的 env/容器固定路径语义、`Table.Pool/PoolIn/AppData(旧)/SpacePools/InScope/InScopeIn`、
`fs.ErrNoMountTable/ErrOutOfScope`；新增/保留错误：`ErrNoSpec/ErrNoMount/ErrAmbiguous/ErrBadUFI/ErrEscape/ErrReadOnly/ErrNoGrant/ErrGrantDenied/ErrNoAppData`。

### 3.3 空间视图（`App.Space`）

```go
view, err := app.Space(space)   // 校验链：ValidSpaceName → spec.spaces → paths.data
errors.Is(err, appsdk.ErrInvalidSpace)        // 400
errors.Is(err, appsdk.ErrSpaceNotAuthorized)  // 403（space ∉ spec.spaces）
errors.Is(err, appsdk.ErrNoSpaceDataDir)      // 500（无 paths.data）
errors.Is(err, appfs.ErrNoSpec)               // 503（daemon 未写 spec）

view.Name() / view.AppDataDir() / view.Ensure()
view.Mounts() / view.Mount(scope)
view.Pools()  // 该空间的 grant-backed mounts
view.Pool(name) // 池只被一个 scope 覆盖时可用；多 scope 覆盖 → fs.ErrAmbiguous
view.LocalPath(pool, rel)
view.LocalPathForUFI(ufi)
```
UFI 解析规则：`/ufi/pool-raw/<pool>/<rel>` → 在（指定空间的）mounts 里按 `pool` 匹配，
用 `root` 裁掉池内前缀（`root==""` 直接用 rel；要求 `rel==root` 或前缀 `root+"/"`），
再拼到 `mount.path` 并做逃逸/软链校验；0 匹配 `ErrNoMount`、多匹配 `ErrAmbiguous`。

### 3.4 app 私有文件落点

- 新文件写 `app.DataDir()`（整个 app 一个）或 `view.AppDataDir()`（每空间一个，`Ensure()` 创建）。
- 旧池内 UFI（如 `/ufi/pool-raw/syspool/.siyouyun/appdata/<app>/...`）只有在某个 mount 覆盖该目录时才可解析；
  正常没有 → **需要一次性迁移或按需重生成**（如封面从媒体重刮），不要假设旧路径仍可用。

## 4. 三个 app 的最小迁移清单

### music（task-30）
- [x] `go.mod` replace 到本 worktree；删除 `contrib/watchdirs` 依赖
- [x] 事件：`application.Events().Subscribe` → `capability.StartFileEventListener(ctx, app.Bus(), opts, handler)`；`Dirs: []string{"/Music"}`、`MediaTypes: audio`、`Actions: add/delete/rename`、`TransportBus`；删除 `/watch/dirs` API
- [x] manifest：`scopes` 去掉 `{value: space}`，保留 `space.music` + `app`；`version` bump（按分支规则）
- [ ] 私有文件（封面缓存）：从池内 `/.siyouyun/appdata/music` 迁到 `app.DataDir()`/`view.AppDataDir()`（旧文件迁移或重生成）
- 注意 writer 输出的 scope 是 core scopeKey（`music`），不是 manifest 的 `space.music`
- [ ] 单测：`internal/testspec` 用真实 spec（JSON + `SYY_RUNTIME_SPEC` + `FromEnv`）已就绪；注意 `runtime.Spec.Bus` 字段名（若改用 struct 构造）
- 已删除的旧用法：`view.Pool("syspool")` 的 appFilesPool 检查（多 scope 同池会 `ErrAmbiguous`）

### movie（task-31）
- [x] `go.mod` replace；`cmd/v2/watch_dirs*` 删除
- [x] 事件改 `capability.StartFileEventListener`（`Dirs: /Movies`，逻辑目录不是 scope key）
- [ ] manifest：去掉 `{value: space}`，保留 `space.movies` + `app`；版本 bump；`contract.open_with` 按需（access: posix / entry "/"）
- [ ] 海报/剧照：新文件写 `app.DataDir()`/`view.AppDataDir()`；旧 `syspool/.siyouyun/appdata/movie` UFI 做一次性迁移或按需重刮，读取失败不要整页报错
- [ ] 多 pool 历史 UFI：只有被当前 scope 授权根覆盖的 pool 可解析；其它旧记录需迁移/重扫；writer 输出的 scope 是 core scopeKey（如 `movies`），不是 manifest 的 `space.movies`
- [ ] 测试注入：`NewWithOptions(Runtime{AppCode, DataDir, SpecPath})` 或 `SYY_RUNTIME_SPEC`+env + `FromEnv`；mount 目录必须真实存在（escape 校验会解析最深存在祖先）
- 保留 movie 既有修复：刮削图片 tmp+fsync+rename

### telebook（task-32）
- [x] `go.mod` replace（含注释：发版前移除 replace）
- [ ] `application.Space(space)` / `SpaceFromHeaders` / `ErrSpaceNotAuthorized` 保持不变；联系人照片 `LocalPathForUFI` 依赖 minictl 写 `space/pool/root` 的 mount
- [ ] manifest：按实际访问目录收窄（只读写 app 私有/DB 则去掉 `{value: space}`；照片需要文件目录则保留具体 `space.<dir>` 并说明）；writer 侧 telebook 的 scope 是 `space`（core scopeKey），root 为空（整空间）
- [ ] `SpaceView.Ensure()` 可删（PG 元数据不依赖它）；若保留只用于私有文件
- [ ] `MINI_APP_PG_DSN` 仍注入、PG schema 逻辑不变
- [ ] 测试注入同上；注意旧 `Runtime{MountTable: ...}` 构造已不可用

## 5. 本地 replace 构建（104 验证方式）

```go
// <app>/go.mod
replace github.com/siyouyun-open/siyouyun_sdk/v2 => /Users/weikeliu/workspace/jzxl/wt-sdk-v2-runtime
```
```bash
cd /Users/weikeliu/workspace/jzxl/wt-faas-apps/app/<music|movie|telebook>
go build ./... && go test ./... -count=1
```
- replace 生效后不需要网络拉 SDK tag；`go.sum` 里已发布的 v2.x 行可保留，发版前把 replace 去掉、改用 SDK tag（提建议 v2.4.0，等 Lead 发布窗口）。
- 当前状态（2026-11-18）：music / movie / telebook 三个 app 在各自 worktree 改动下**均已 `go build ./...` 通过**（replace 到本 worktree）。
- 103 真实数据面需等 minictl writer（task-34）按第 2 节字段写出 spec；在那之前单测用第 6 节方式构造 spec。

## 6. 单测注入方式

```go
// 方式 A：直接给 Runtime 一个临时 spec（推荐单测）
specPath := filepath.Join(t.TempDir(), "runtime.json")
os.WriteFile(specPath, specJSON, 0o600)
app := appsdk.NewWithOptions(appsdk.Runtime{
    AppCode: "music", DataDir: spec.Paths.Data, SpecPath: specPath,
})

// 方式 B：与生产一致（SYY_RUNTIME_SPEC + 环境身份）
t.Setenv(runtime.EnvSpecPath, specPath)
t.Setenv(protocol.EnvRuntime, "native")   // 以及 APPCODE/MINI_APP_ID/... 需要的最小集合
app := appsdk.New()

// 方式 C：`appsdk.FromEnv()` 的 Runtime 直接用于构造
rt := appsdk.FromEnv()
table, err := appfs.LoadFrom(rt.SpecPath)
```
注意：
- spec 里的 `paths.*` 与 `mounts[].path` 必须是**真实存在**的临时目录（escape 校验会解析最深已存在祖先），否则 mount 解析会拒；
- fixture 可复制 `runtime/testdata/runtime-spec-v1-multispace.json` 后把绝对路径替换成 `t.TempDir()`；
- `runtime.Spec` 字段名注意：`Bus`（不是 `BusConfig`）、`Mounts []Mount`、`Grants []Grant`、`Mode`/`Op` 枚举常量在 `runtime` 包。

## 7. 已知限制 / 待办

1. **minictl writer（task-34）已实现**：按第 2 节写 `spaces` + `mounts{space,pool,root}` + `grants[].space`，逐空间查询/挂载；两份真实 sample 已作为 golden fixture 纳入集成测试（`TestMultiSpaceSpecIntegration` 三个 case 全绿）。
2. **协议同步**：SDK 已删除 `protocol/mounts.go`；host 侧 `pkg/protocol` 的 `contract_test.go`/`testdata/contract_vectors.json` 仍带 `mount_table` 段，`scripts/check-protocol-sync.sh` 会报差异，需 host 侧同步删除。另 SDK `protocol/env.go` 仍保留几个 SDK 本地开发用的 env 常量（`MINI_APP_INSTALL_ROOT/DATA_DIR/...`）、host 侧没有，属仓库拆分后的既有差异，逐字节同步需两边先统一定夺（详见交付报告）。
3. **SDK tag**：建议 v2.4.0，由 Lead 定发布窗口；本 worktree 不 commit/push/tag。
4. **容器形态**：路径 `/lacx/run/runtime.json`；容器 writer 可选加同样 `space/pool/root` 字段（V3 多空间暂不需要）。
