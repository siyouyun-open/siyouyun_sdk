# Native application SDK (`github.com/siyouyun-open/siyouyun_sdk/v2`)

> This is the SDK's own manual: it is published with the SDK repository
> `github.com/siyouyun-open/siyouyun_sdk` (branch `v2`, module path
> `github.com/siyouyun-open/siyouyun_sdk/v2`, released as `v2.1.0`). All packages sit at the
> **repository root** — there is no `appsdk/` prefix. The bus, capability, event and
> mount-table behaviour described here is the contract implemented in `protocol/`.
>
> The platform runtime repository (`mini-runtime`) keeps a copy of this manual at
> `docs/technical/APPSDK.md` for the runtime-side context (daemon / minictl expectations);
> change SDK behaviour here first, then sync that copy. Paths that point into the runtime or
> FaaS repositories are labelled as such throughout. `pkg/protocol`, `pkg/types` and
> `module/minictl` below are the platform runtime repository's paths, not this repository's.

`github.com/siyouyun-open/siyouyun_sdk/v2` 是 **AppStore native（宿主机进程）应用**接入 minictl 能力总线的官方
客户端。它的目标只有一个：让应用用一段普通 Go 代码就能调用平台能力，而不用自己实现
UDS 协议、身份令牌、并发应答匹配、事件订阅与断线重连。

它是**总线客户端**，不是应用框架：不要求 Web 框架、不规定目录结构、不在初始化时
`panic`，也不接管进程生命周期。

---

## 0. Open-with POSIX file grants

Open-with file grants are a runtime contract, not an SDKv2 capability.
Applications receive a `path` query parameter pointing at
`/lacx/userfs/grants/<grantPrefix>/<basename>` and read it with ordinary POSIX
calls through the stable FUSE grants root. The old grant URL and
`/syy/v2/app/fs/grant/*` HTTP endpoints have been removed.

SDKv2 does not expose an open-with file API and must not be used to translate a
grant path through the mount table. See:

- [POSIX Open-With](POSIX-OPEN-WITH.md)
- [Open-With POSIX Migration Specification](POSIX-OPEN-WITH-MIGRATION.md)

## 1. 模块结构

```text
github.com/siyouyun-open/siyouyun_sdk/v2/   ← SDK 仓库的根目录
├── appsdk.go            运行环境解析 + 客户端装配（包根，Go 包名仍是 appsdk）
├── space.go             空间校验与空间视图（见 §12）
├── protocol/            零依赖线协议与能力目录（**与 host 的 pkg/protocol 逐字相同**，见下）
├── appconst/            响应码 / 身份头名 / 空间名（对应旧 SDK 的 pkg/const）
├── apperr/              国际化错误（i18n key + 参数）
├── paging/              最小分页模型（零依赖）
├── contrib/restjson/    `{code,data,msg}` 响应信封（单独模块：它依赖 iris，tag `contrib/restjson/v0.1.0`）
├── fs/                  挂载表解析 + 本地 POSIX 数据面（不是总线能力，见 §5.1）
└── capability/          按域封装的强类型能力（KV、兴趣点监听、守护进程…）

siyouyun_faas/examples/sdk-demo/   **两个示例应用共用一份源码**（见 §11；FaaS 仓库）
├── main.go / checks.go / static/   共享代码与自测面板
├── native/                     appid …-native，包源 version: native
├── container/                  appid …-container，包源 version: v3
└── Makefile                    container（编容器二进制）/ pack / deploy

siyouyun_faas/scripts/e2e-appsdk.sh  真机端到端回归（隔离 daemon，两种形态都部署；FaaS 仓库）
```

依赖方向：

```text
native app   ┐
             ├→  appsdk  →  capability  →  bus  →  protocol
容器 app     ┘                                          ▲
                                                        │ 逐字相同的两份副本
                                  minictl / pkg-types → pkg/protocol
```

`protocol` 里的东西（作用域值语法、挂载表字段、总线帧与消息类型、能力模板名、
响应码）是**运行时与应用必须逐字一致**的契约，因此 host 侧（`pkg/protocol`）与
SDK 侧（`protocol`）各留一份**逐字节相同的包**：

- 为什么不用一个共享模块：SDK 要能 0 外部依赖地独立存在（整包可搬走/可被复制进
  任意应用仓库），host 也不该为了一个契约去依赖应用侧的 SDK。
- 为什么不怕分叉：三道闸——`ContractVersion`（改语义必须显式提高）、两份共用的
  `protocol/testdata/contract_vectors.json`（谁改行为谁先红）、host 仓库（platform runtime）的
  `scripts/check-protocol-sync.sh`（按 `SDK_DIR` 或同级 checkout 找 SDK 后逐字节 diff，
  只 checkout 了 runtime 一个仓库时打印 SKIP）与 `pkg/types` 里的一致性测试
  （逐字节 diff，`go test ./...` 与 CI 都会跑）。
- 改契约的顺序：先改 host 的 `pkg/protocol`（host 是权威）→ 在 host 仓库跑
  `scripts/check-protocol-sync.sh --write` 同步到 SDK → 语义变化同时提高 `ContractVersion`
  并更新向量文件。

daemon 侧的 JSON 编解码（`capability/codec_json.go`，在 platform runtime 仓库）与 WS 帧结构体直接用协议包，
并有 parity 测试保证模板目录与 daemon 运行时注册表逐条对齐。

SDK 的第三方依赖只剩 `gorilla/websocket`（总线传输，纯 Go 无 cgo）。原先的进程内
SQLite 层 `module/appsdk/db`（当时还在 runtime 仓库）**已被删除，没有迁进本仓库**：
四个应用回迁 PostgreSQL 后它成了死代码，连同 `modernc.org/sqlite` 一并移除。
本仓库里没有任何 `db` 包。

---

## 2. 快速开始

```go
package main

import (
	"context"
	"log"

	"github.com/siyouyun-open/siyouyun_sdk/v2"
	"github.com/siyouyun-open/siyouyun_sdk/v2/capability"
)

func main() {
	app := appsdk.New() // 读 MINI_APP_* 环境；不建连、不 panic
	defer app.Close()

	ctx := context.Background()

	// 应用维度 KV：跟随应用，与用户无关。
	if err := app.KV().App().Put(ctx, "config", "theme", "dark"); err != nil {
		log.Printf("kv put: %v", err) // 总线不可用时降级，而不是崩溃
	}

	// 用户维度 KV：native 必须给出作用域（见 §4）。
	userKV := app.KV().User(capability.Scope{Username: "alice", Namespace: "main"})
	value, found, err := userKV.Get(ctx, "config", "theme")

	// 兴趣点监听：纯总线模式（应用不连 NATS）。
	listener, err := capability.StartFileEventListener(ctx, app.Bus(), capability.FileEventListenerOptions{
		Dirs:      []string{"/Photos"},
		Transport: capability.TransportBus,
	}, func(ev capability.FileEvent) error {
		log.Printf("file %s: %s", ev.ActionName, ev.UFI)
		return nil
	})
	if err == nil {
		defer listener.Close()
	}
}
```

`appsdk.New()` 只解析环境与装配客户端；首次调用能力时才建连。因此即使平台总线暂时
不可用，应用也能正常启动并自行降级。

---

## 3. 传输与身份

平台给 native 进程注入三个总线入口，SDK 按固定优先级选择：

| 入口 | 环境变量 | 用途 |
| --- | --- | --- |
| UDS | `MINI_APP_BUS_SOCKET` | **首选**：RPC + 订阅 + 事件，本机最快，且只有它能携带 `custom.*` 的 `signature` |
| HTTP | `MINI_APP_BUS_HTTP` | 兜底 RPC，以及文件/流式能力的唯一通道（JSON 帧上限 64 KiB） |
| WS | `MINI_APP_BUS_WS` | 没有 UDS 时的事件与 RPC 通道 |

身份由服务端解析，客户端**不声明** `appCode` / `namespace`：

- UDS 的身份来自 socket 与实例的绑定；
- HTTP / WS 上 SDK 自动带 `X-Mini-Caller-Token`（`MINI_APP_CALLER_TOKEN`），daemon
  用它恢复实例身份。这枚令牌只用于**识别**调用者，不是授权凭据。

Containers expose only one entry point, UDS (`/lacx/bus/app.sock`, bind-mounted at deploy
time), and identity likewise comes from the socket and the source address; containers have
no caller token and do not need one. Client code does not need to branch for this —
`Runtime.BusSocket` is a usable absolute path in both forms.

`app.State()` 返回当前传输与连接状态，适合放进应用自己的健康检查：

```go
state := app.State() // {Transport: "unix", Connected: true, LastError: ""}
```

三条传输都在真机上验证过（FaaS 仓库的 `siyouyun_faas/scripts/e2e-appsdk.sh`：native 形态 UDS / HTTP / WS 各连
一次真实 daemon 并完成能力调用；容器形态验证 UDS）。HTTP 通道依赖两件容易漏掉的事，已在 daemon 侧修好并写了
回归：`POST /api/v1/capability/call` 在回环上接受 native 调用者令牌，且 daemon
自提供的能力（KV、Core/OS 代理）在这条通道上同样会回退执行。

---

## 4. 用户作用域

容器应用部署在某个用户空间里，daemon 从连接命名空间就能推导出用户名与空间。
native 是"装一份、服务多空间"的共享宿主机进程，连接命名空间恒为 `default`，推不出
用户，因此**用户维度能力必须显式给出作用域**：

```go
// 个人空间：{username}-{namespace}
app.KV().User(capability.Scope{Username: "alice", Namespace: "main"})

// 群组空间：{groupName}-{namespace}
app.KV().User(capability.Scope{Username: "alice", GroupName: "group", Namespace: "team"})
```

daemon 会用实例的授权空间集合校验它（`WebNamespaces`，由 AppStore 按登录用户空间补齐）；
未授权的空间直接拒绝，不会静默写到别的用户空间。

作用域留空表示"由 daemon 从连接推导"，这只在容器场景成立——native 留空会得到明确的
`must pass username and namespace` 错误。

两种形态都可以直接用 `app.Runtime.DefaultScope()`：

| 形态 | `DefaultScope()` | 含义 |
| --- | --- | --- |
| native | 空 | 推不出用户，必须由调用方显式给出 |
| 容器 | `{Username: <user>, Namespace: <用户空间后缀>}` | 部署它的那个空间 |

注意容器侧返回的 `Namespace` 是**用户空间后缀**（`main`）而不是部署空间全名
（`admin1-main`）：能力接口的语义是 `{username}-{namespace}`，传全名会拼成
`admin1-admin1-main`，daemon 会直接拒绝。`Runtime.Namespace` 才是部署空间全名。

容器的空间是部署时定的，daemon 会按连接命名空间**校验**它：显式传一个别的空间会被
拒绝——容器只被部署在一个空间里，`{groupName||username}-{namespace}` 必须能拼回
连接命名空间。这条校验是补上的（见 §11.3）。

### 4.1 怎么判断"这个空间授权了吗"（应用侧）

**不要用挂载表判断。** native 的存储池来自 `StorageNamespace ∪ WebNamespaces`：安装
空间即使还不在 `WebNamespaces` 里，也会出现在挂载表里，但用户维度能力**仍然会被
拒绝**。真机回归里就是拿挂载表当依据，得出了与 daemon 相反的结论。

可靠的做法只有一个：**发一次只读、无副作用的用户维度调用去问 daemon**。授权通过时
它只是"没查到"，未授权时是明确的 `not authorized`：

```go
_, _, err := app.KV().User(scope).Get(ctx, "probe", "authorization-probe")
authorized := err == nil                          // 或：!strings.Contains(err.Error(), "not authorized")
```

`appsdk-demo` 面板的「自动判断」就是这么做的，结论会写进报告的
`params.scopeAuthorized` / `params.scopeBasis`，便于复盘。

作者侧还要知道一件事：**未授权时不能假设错误文案**。`not authorized` 是授权校验的
信号，其它错误（schema 不存在、网络失败）都不是——把二者混起来会把真故障当成"正常
拒绝"。

> 上面说的是**总线能力**的授权，判据只有 daemon。挂载表回答的是另一个问题：
> "这个空间的数据我能不能**直接碰**"（本地 POSIX / 元数据库）。后者用挂载表判断
> 是正确的，而且必须判断——见 §12.1。两件事不要互相替代。

---

## 5. 能力封装

当前已封装：

| 能力 | API | 传输 |
| --- | --- | --- |
| 本地文件 | `app.FS().Pool(...)` / `PoolIn` / `AppData` | **本地 POSIX**，不走总线 |
| 用户 KV | `app.KV().User(scope).Get/Put/List/Delete` | 总线 `system.kv.*` |
| 应用 KV | `app.KV().App().Get/Put/List/Delete` | 总线 `system.kv.sys.*` |
| 兴趣点监听 | `capability.StartFileEventListener(ctx, app.Bus(), opts, handler)` | NATS 共享流 consumer（nats）或总线 `system.interest.event`（bus），见 §13 |
| 任意事件 | `capability.Subscribe(ctx, app.Bus(), topics, handler)` | 见 `protocol.Topic*` |
| 守护进程 | `app.Daemon().Health(ctx)` | 总线 `system.daemon.health` |
| 媒体信息 | `app.Media().FFmpegInfo(ctx, scope, ufi)` | 总线 `system.media.ffmpeg.info`（daemon→OS） |
| 设备端点 | `app.Device().ServiceEndpoints(ctx)` | 总线 `system.device.service-endpoints`（daemon→OS） |
| 空间与配额 | `app.Storage().UserSpaces(ctx, scope)` | 总线 `system.storage.user-spaces`（daemon→OS） |
| 回收站 | `app.Storage().Trash(ctx, scope, ufi)` | 总线 `system.fs.trash`（daemon→Core） |
| 机器人消息 | `app.Message().Send(ctx, scope, content, reply)` | 总线 `system.message.send`（daemon→Core） |
| 定时任务 | `app.Schedule().AddOnce/AddCron/UpdateOnce/UpdateCron` | 总线 `system.schedule.*`（daemon→Core） |
| 直通调用 | `app.Capabilities().Call(ctx, &bus.Request{...}, out)` | 任意模板 |

平台能力的完整清单在 `protocol.Catalog()`（编译期可见），运行时可用情况以
daemon 的 `minictl capability list` / `capability/discover` 为准。

### 5.0 还缺什么

对照老 `siyouyun_sdk` 的能力面，**Core/OS 的 HTTP 接口已经全部有对应模板**。仍缺的
是那些不是"HTTP 代理"、需要新基础设施的能力：

| 仍缺 | 为什么还没做 |
| --- | --- |
| AI 推理（人脸/文本/图像向量） | 上游是 gRPC（`:40051`），要么让 daemon 依赖该 gRPC 服务，要么做成 provider App；是一个独立决策 |
| 任务中心 | 上游是 NATS `siyou_task.*`，且 OS 侧实现不在本仓库；需要任务存储 + 调度，工作量大 |
| 定时任务/消息的**事件生产** | 平台侧要真正按 cron 触发并投递到 `system.event.schedule`；当前只做到"注册任务" |
| 系统事件（`app_migration` / `app_cleanup`） | 上游是 NATS JetStream 流，需要新的生产/消费路径 |
| 租户数据库（`AppDSN` / `Exec`） | **应用自己连设备本地 PostgreSQL**：每个用户空间一个 schema（`faas_<owner>_<namespace>`），每应用一个连接池、事务内 `SET LOCAL search_path`。做法与理由见 platform runtime 仓库的 `docs/specs/faas-new-sdk-migration.md` §5。`AppDSN` 是数据库凭证，不适合经总线下发；过渡期用过的池内 SQLite 层（`module/appsdk/db`）已删除，**没有迁进本仓库** |
| 应用元数据 `system.app.info` | 刻意不暴露：`appAddr` 已由环境变量注入，剩下的 `appDSN` 同上 |

未封装的能力都可以用直通调用；缺的这几项需要先决定架构（daemon 直连 vs provider App）。

### 5.1 Local files (not a bus capability)

Storage permissions are realised as "the runtime hands the directory to the process", so file access is a **local POSIX call**:
there is no HTTP round trip, and `mmap`/`sqlite`/`sendfile` all work. Applications address files by **logical scope** or
`(space, pool, relative path)` / UFI, and the path is always resolved by **RuntimeSpec v1**:
inside a container it is a `/lacx/...` logical path, for native it is the host mount target — the SDK code is identical
(`SYY_RUNTIME_SPEC` points at the spec; in a container it is fixed to `/lacx/run/runtime.json`).

```go
table, err := app.FS()                 // parse the RuntimeSpec and build the data plane

mount, err := table.Mount("music")                // unique scope (core scopeKey)
mount, err = table.MountIn("alice-main", "music") // multi-space instances must pass it explicitly
data, err := mount.ReadFile("2024/a.mp3")
f, err := mount.OpenFile("cache.db", os.O_RDWR|os.O_CREATE, 0o644)

appData, _ := table.AppData()          // paths.data (app private directory, no grant concept)
local, _ := table.LocalPathForUGN("alice", "alice", "main", eventUFI) // UFI -> local path
```

Key points:

- the spec's `mounts[]` carries `scope / path / mode / grantId`, and multi-space native mounts also carry
  `space / pool / root`: `LocalPathForUGN/LocalPathForUFI` use exactly these three to map
  `/ufi/pool-raw/<pool>/<rel>` back to a path relative to that scope's mount;
- the top-level `spaces` is the set of spaces the instance is authorised to serve; the same scope has one mount per
  space, so calling `table.Mount(scope)` without a space returns `ErrAmbiguous`;
- read-only/read-write is decided jointly by `mode` (`ro`/`rw`) and `grants[].ops`: the mount's mode must be
  allowed by the grant, otherwise spec validation fails; write operations return `ErrReadOnly` early;
- `..`, absolute paths and symlink escapes are rejected; native is not a sandbox against the host — the mount root is the boundary;
- the daemon regenerates the spec on startup / after authorisation changes, and `app.ReloadFS()` re-reads it
  (the process environment is fixed at startup; the old `mounts.json` / `MINI_APP_MOUNT_TABLE` /
  `NATIVE_STORAGE_POOL_*` are deleted with no fallback);
- cross-pool moves handle `EXDEV` automatically (copy + delete), with the same semantics as a local filesystem.

The full contract lives in the platform runtime repository at `docs/specs/file-provider-runtime-spec-20261117.md`
and `docs/specs/runtime-spec-v1.schema.json`.

Capabilities that are not wrapped can be called directly through passthrough:

```go
var spaces []map[string]any
err := app.Capabilities().Call(ctx, &bus.Request{
	Template: protocol.TemplateStorageSpaces,
	Payload:  map[string]string{"username": "alice"},
}, &spaces)
```

自定义域能力必须带上从能力发现拿到的准确签名：

```go
err := app.Capabilities().Call(ctx, &bus.Request{
	Template:  "custom.example.album.search",
	Signature: "<signature from discovery>",
	Payload:   map[string]any{"q": "cat"},
}, &result)
```

---

## 6. 事件语义

普通总线订阅推送是 **at-most-once、不保证顺序、不是持久日志**：

- 连接中断期间的推送不会补发；
- 订阅在重连后会被幂等重放，但重放会给服务端一次重复订阅（服务端幂等）；
- 事件通道是**有界的**，满了会丢弃并打日志——不能让慢消费者把 reader 拖死，
  否则所有在途调用会一起卡住。

兴趣点监听是例外：NATS 模式由 OS 共享流的 durable consumer 提供 at-least-once（应用
ACK/NAK，见 §13）；纯总线模式由 daemon 拉取，SDK 通过 `system.interest.ack` 确认/重试。

因此 handler 必须幂等；需要强一致时自行对账（例如按事件里的 UFI 重新拉取状态）。

订阅时只订阅基础主题（`protocol.Topic*`），daemon 会展开成调用方自己的作用域
（例如纯总线兴趣点事件的 `system.interest.event.<app_id>`）。同一 `bus.Client` 上的多个
capability 订阅共享一个事件泵并按 topic 扇出，不会互相抢事件。

纯总线兴趣点事件（`transport:"bus"`）的投递确认在 daemon 侧：daemon 拉取 OS 消费者、
扇出到总线后等待应用 `system.interest.ack` 才 ACK，应用侧没有 NATS 消息可 ACK/NAK。NATS 模式（`transport:"nats"`）
则由应用自己对共享流消息 ACK/NAK，并把消费结果回写 `os.interest.result`（见 §13）。

---

## 7. 错误处理

SDK 区分三类失败，应用应区别对待：

| 错误 | 判断方式 | 含义与处理 |
| --- | --- | --- |
| 能力失败 | `*bus.CallError`（含 `Code`） | daemon 明确拒绝：模板不存在、参数非法、provider 失败。先修输入，别无脑重试 |
| 超时 | `errors.Is(err, bus.ErrTimeout)` | 30s 内没有应答。可重试，但注意副作用型能力要幂等 |
| 连接失败 | 其它（底层 `net.Error` 等） | SDK 已自动重连并重试一次；持续失败说明总线不可用，走降级路径 |

```go
var callErr *bus.CallError
switch {
case errors.As(err, &callErr):
	log.Printf("能力被拒绝: %s", callErr.Message)
case errors.Is(err, bus.ErrTimeout):
	log.Print("超时")
default:
	log.Printf("总线不可用: %v", err)
}
```

---

## 8. Runtime environment helpers

```go
rt := app.Runtime
rt.AppID          // com.siyouyun.album
rt.AppCode        // album
rt.InstanceID     // native-album-<hash>
rt.Port           // listening port of this app (assigned by the daemon)
rt.DataDir        // platform-managed data directory; empty when storage.app.file is not declared
rt.InstallRoot    // versioned install directory (do not use as a persistent directory)
rt.Spec           // parsed RuntimeSpec v1 (nil during local development)
rt.SpecPath       // spec file location (for troubleshooting / ReloadFS)

mounts := rt.Spec.Mounts   // raw view of scope -> path/mode/grant
spaces := rt.Spec.Spaces   // spaces the instance is authorised to serve
```

An application **must** read its listening port from `MINI_APP_PORT` / `MINI_APP_ENDPOINT`, return
2xx from `native.health_check.path`, and write persistent data under `RuntimeSpec.paths.data`
(`app.DataDir()` / `SpaceView.AppDataDir()`); do not write new application-generated files into the user pool UFIs.
The old `rt.MountTable` / `rt.StoragePools` / `FindStoragePool` are deleted.

---

## 9. 本地开发与测试

`appsdk.New()` 在没有平台环境时也能创建实例，只是没有总线入口：

```go
app := appsdk.NewWithOptions(appsdk.Runtime{},
	appsdk.WithBusHTTP("http://127.0.0.1:8080"),
	appsdk.WithCallerToken(os.Getenv("MINI_APP_CALLER_TOKEN")),
)
```

单元测试里可以用 `httptest` 伪造 HTTP 能力端点，或用临时 UDS 服务端验证请求形状——
SDK 自身测试就是这么做的（`bus/client_test.go`、`capability/kv_test.go`）。

跑全部 SDK 单元测试：`go test ./...`（本仓库没有 Makefile；`make test-appsdk` 是 platform
runtime 仓库的聚合目标）。

### 9.1 真机回归与自测面板

`siyouyun_faas/scripts/e2e-appsdk.sh`（在 FaaS 仓库，不在本仓库）在 NAS 上起一个**完全隔离**的 daemon（`:18081`、`/tmp/nles`
下的 base_dir / 用户数据根 / runc 根目录 / bind 挂载池），把**同一份源码**的两种形态
都部署进去，再用各自的 HTTP 接口逐项断言 SDK 的真实行为：

| 阶段 | 形态 | 空间 | 数据面 |
| --- | --- | --- | --- |
| 一 / 二 | native 宿主机进程 | `default`（授权空间 `admin1-main`，再补 `demo1-private`） | 注入的绝对路径（`/tmp/nles/home/...`） |
| 三 | 容器（v3 / runc） | `admin1-main` | 容器内 `/mnt/userfs/...`，经网关访问 |

断言覆盖：总线三条传输（容器侧只有 UDS）、身份与作用域校验、Core/OS 代理、挂载表解析
与本地 POSIX 读写、UFI 映射、越界拒绝、跨挂载移动的 `EXDEV` 兜底、兴趣点监听（NATS / 纯总线两种
transport），以及
"授权空间变化 → daemon 重算存储池 → 重写挂载表 → `App.ReloadFS()` 看到新空间"。

容器阶段额外验证：网关 → 容器路由与就绪探测（`/web/<hash>/health`）、池路径确实是
容器内的 `/mnt/userfs/...` 而不是宿主机路径。

```bash
# 全自动回归（跑完清理；脚本在 siyouyun_faas 仓库，需要在其中执行）
siyouyun_faas/scripts/e2e-appsdk.sh --host root@10.0.1.156

# 保留环境并打印两种形态各自的面板地址
siyouyun_faas/scripts/e2e-appsdk.sh --host root@10.0.1.156 --serve
# → native 面板：http://10.0.1.156:18081/web/<native_hash>/
# → 容器面板：http://10.0.1.156:18081/web/<container_hash>/
```

脚本用两个环境变量定位仓库：`MINI_RUNTIME_DIR` 指向 platform runtime 仓库（提供
`minictl` / `mini-packer`，默认 `$REPO_ROOT/../mini-runtime`，不存在则报错退出）、
`MINI_APP_SDK_DIR` 指向本 SDK 的检出目录（默认 `$REPO_ROOT/../siyouyun_sdk`，会被写成
示例应用 `go.mod` 里的绝对 `replace`）。示例应用在 `siyouyun_faas/examples/sdk-demo`。

面板（`static/index.html`）打开就会自动跑一遍无副作用的模块，结论直接可见：

- 顶部一条结论横幅（通过 / 失败 / 跳过 / 耗时）+ 运行形态、appid、空间等徽章；
- 每个模块一张卡片，卡片头就有状态徽章与 `通过/总数`，展开是逐条断言与详情，
  失败的模块默认自动展开；
- 有副作用的模块（发消息、建定时任务、清回收站）单独一组，点运行时会先 `confirm()`；
- 参数收在「测试参数」折叠区，填过一次会存在浏览器里。

用户维度能力的作用域是 `namespace = {user}-{ns}`（例如 `admin1` + `main` →
`admin1-main`，即授权空间列表里的名字）。native 形态**必须**填；容器形态留空也可以
（daemon 从连接推导）。

断言方向**不需要人判断**：参数区是一个三态选择，默认「自动判断」——运行时先用一次
只读探针问 daemon（§4.1），再决定「这些能力应当通过」还是「应当被拒绝」。另外两个
选项是"强制已授权 / 强制未授权"，只在故意验证某条路径时用。此外还有一条不依赖任何
参数的确定性用例 `kv.user.foreign_scope_rejected`：合成一个不可能被授权的空间，它
必须被拒绝——这正是当初暴露"容器可以读写任意用户 KV"的那条断言。

**复制 JSON 回来排查**：面板提供「复制 JSON / 只复制失败项 / 下载 JSON」。报告自带
`schema`（当前 `appsdk-demo/2`）、`summary`、按模块聚合的 `modules` 与扁平的 `checks`
（每条含 `module` / `name` / `status` / `ok` / `skipped` / `detail`），所以拿到 JSON
就能直接定位是哪一层出的问题。

> 复制的实现（踩过的坑，改之前先读）：面板常通过 `http://<ip>:<port>` 访问，不是安全
> 上下文，`navigator.clipboard` 在那些浏览器里是 `undefined`；而 `execCommand('copy')`
> 有两个前提——**同步调用**（中间插入 `await` 会丢掉用户手势）和**选中的元素必须已渲染**。
> 最初把 JSON 放在折叠的 `<details>` 里，收起时元素没有布局，`select()` 选不中任何内容，
> 而 `execCommand` 仍然返回 `true`，表现就是"提示已复制、剪贴板却没变"。
>
> 现在按这个顺序：① 临时 textarea（屏幕外但已渲染）→ `execCommand`；② 安全上下文再走
> `navigator.clipboard`；③ 都失败就展开 JSON 面板、滚进视野并全选，让用户按 ⌘/Ctrl+C。
> 三条路都失败也不会静默：提示里会写"已全选"。

> 示例应用放在 FaaS 仓库的 `siyouyun_faas/examples/sdk-demo` 下，但 **native 形态**的
> manifest 必须是 `version: native`（`container/` 子目录里才是 `version: v3`）。两种形态共用一份
> `main.go` / `checks.go` / `static/index.html`，差异只在包源目录与运行环境。

用完清理隔离环境：

```bash
ssh root@10.0.1.156 'rm -rf /tmp/nles'
```

### 9.2 已知平台侧缺陷（不是 SDK 的问题）

面板上三个带副作用的模块里，两个在真机上必然报错，原因都在平台侧，记录在此以免误判：

| 模块 | 现象 | 真实原因 |
| --- | --- | --- |
| `schedule.once.create` | Core 返回 `invalid input syntax for type json (SQLSTATE 22P02)`，日志指向 `modules/syycore/internal/schedule/db.go:18` 的 `db.Create` | `sysmodel.AppScheduleEvent.Payload` 是 `[]byte` 而列类型是 `jsonb`。GORM 的 postgres driver 走 pgx，`[]byte` 按 **bytea** 文本（`\x7b22...`）下发，`jsonb` 直接拒收。与客户端编码无关：Core 的 Go 模型要求 base64，daemon 发的就是 base64，解出来是合法 JSON，仍然失败。`siyou_system.siyou_app_schedule_event` 当前 **0 行**，说明这条路径从未成功过（旧 SDK 更早就因 body 里缺 `namespace` 被拒）。修法在 Core：字段改成 `json.RawMessage` / `datatypes.JSON`（此时 daemon 的 `scheduleBody.Payload` 要同步从 `[]byte` 改成 `json.RawMessage`），或把列改成 `text`。 |
| `message.send` | Core 返回 `message robot not exist` | 示例应用只在隔离 daemon 里注册，Core 侧没有它的应用机器人。调用链（总线 → daemon 代理 → Core）本身是通的，换成 Core 里真实注册过的应用即可通过。 |
| `fs.trash` | 未填 `ufi` 时显示"跳过" | 有意为之：回收站会真的移动文件，不做隐式猜测。 |


---

## 10. 扩展新能力

1. 在 `protocol/templates.go` 加模板常量与目录条目；
2. 在 daemon 的 `pkg/types` 注册 schema（daemon-provided 或 provider-backed）；
3. 在 `capability/` 加一个文件封装入参与应答；
4. 跑 platform runtime 仓库的 `go test ./module/minictl/...`——`protocol_catalog_test.go` 会强制两侧对齐。

不要在应用里手写模板字符串：常量在 `protocol`，拼错会在编译期暴露。

---

## 11. 一份代码，两种运行形态

SDK 的目标之一是让**同一份应用代码**既能作为 native 宿主机进程、也能作为容器运行。
统一的不是"环境变量名"，而是 SDK 暴露的 `Runtime` 语义；两套平台变量在
`FromEnv` 里被归一化。

> 注意区分两件事：**运行形态**（native/容器，由 SDK 归一化）与**应用身份**
> （appid，是两个不同的应用）。示例里刻意做成两个 appid —— 这样它们才能在
> AppStore 里各自独立安装、升级、卸载；共用源码是靠"同一份 Go 模块 + 两个包源
> 目录"，而不是靠"同一个 appid 打两个包"。

### 11.1 两套环境契约

| | native（宿主机进程） | 容器（v3 / runc） |
| --- | --- | --- |
| 运行类型 | `MINI_APP_RUNTIME=native` | `APP_RUNTIME=RUNC` |
| 应用身份 | `MINI_APP_ID` + `MINI_APP_CODE` | `APPCODE`（取值就是 app id） |
| 实例 ID | `MINI_APP_INSTANCE_ID` | 无（由部署生成，不注入进程） |
| 空间 | `MINI_APP_NAMESPACE` 恒为 `default` | `MINI_NAMESPACE=admin1-main`，用户名 `MINI_USERNAME=admin1` |
| Bus UDS | `MINI_APP_BUS_SOCKET` (absolute path) | fixed `/lacx/bus/app.sock` (bind-mounted at deploy time) |
| 总线 HTTP/WS | 注入 `MINI_APP_BUS_HTTP` / `MINI_APP_BUS_WS` | **不注入**（只有 UDS） |
| 调用者令牌 | `MINI_APP_CALLER_TOKEN` | 无（身份来自 socket 与源地址） |
| 监听端口 | `MINI_APP_PORT` / `MINI_APP_ENDPOINT` | 不注入；由 manifest 的 `network.http` 声明，应用自己知道 |
| Data-plane contract | `SYY_RUNTIME_SPEC` points at `<root>/<appcode>/run/runtime.json` | fixed `/lacx/run/runtime.json` (read-only bind) |
| Storage paths | not injected (the old `NATIVE_STORAGE_POOL_*` / mount table are deleted) | not injected; the whole data plane comes from the RuntimeSpec `mounts[]` |

`Runtime` 上的对应关系：`IsNative()` / `IsContainer()` 判断形态，`DefaultScope()`
给出各自的作用域（§4）。**不要**用"有没有某个变量"来判断形态——判据是 `APP_RUNTIME`。

### 11.2 Data plane: RuntimeSpec v1 is the single source of truth

Local file access in both forms is unified by the `fs` package: applications address files by logical scope or
`(space, pool, relative path)` / UFI, and the RuntimeSpec's `mounts[]` resolves that into an **absolute path the current
process can open directly**, after which they are ordinary POSIX calls. The only difference is how the spec is delivered
and the path prefix (see §11.1); application code needs no branch. The old `mounts.json` contract (`MINI_APP_MOUNT_TABLE`
/ the fixed container `mounts.json` / `NATIVE_STORAGE_POOL_*`) was removed with this migration and is not supported.

### 11.3 已知差异与缺口

- **容器没有 HTTP/WS 总线入口**：容器只保证 UDS。要补上需要平台在部署时注入
  一个容器可达的地址（网桥网关 `172.19.0.1:<daemon 端口>`），目前没有做。
  示例应用的 `bus.http.*` / `bus.ws.*` 断言在容器形态下会明确显示"跳过"。
- **容器不注入端口**：端口来自 manifest 的 `network.http`，平台不告诉进程。
  示例应用用 `APP_PORT`（自己的 `start.sh` 约定）拿到它。
- **容器作用域授权是后补的**：daemon 原先只"给缺省字段填空"、不校验客户端显式传的
  UGN，于是部署在 `admin1-main` 的容器传 `username=demo1/namespace=private`
  （甚至一个根本不存在的空间）都能读写成功——等于容器可以冒充任意用户。现在
  `fillUGNFromBusNamespace` 会要求
  `{groupName||username}-{namespace}` 能拼回连接命名空间，否则以
  `namespace ... is not authorized for this container` 拒绝。
- **default 空间的共享容器 fail closed**：`app.default.shared` 的应用落在 `default`
  空间。native 共享实例按 `WebNamespaces` 逐个校验空间，容器则**没有任何**可用的
  用户身份来源，因此容器在 `default` 空间下传来的 UGN 一律拒绝
  （`shared default-space container must not pass a user scope`）。
  这条以前是放行的，等于落在 `default` 的容器可以读写任意用户空间的 KV。
  要重新放开，必须由平台先给出"这个应用被授权了哪些用户空间"的显式来源
  （manifest 权限 + 管理员白名单），而不是相信请求里的 `username`/`namespace`。
  宿主机上的其它调用方（AppStore / CLI）不受影响：daemon 用容器 ID 是否存在来区分
  "容器"与"本机组件"。

### 11.4 部署两个形态（mini-packer）

```bash
cd siyouyun_faas/examples/sdk-demo
make pack-container          # v3 没有构建钩子，必须先把二进制编进 container/
mini-packer deploy -s native       # → com.nas.common.sdk-demo-native
mini-packer deploy -s container    # → com.nas.common.sdk-demo-container
# 或者一次做完：make deploy
```

两边都是 `mini-packer login <nas-ip>` 之后的常规 push 流程，装完就是 AppStore 里
两个各自独立的应用。

**native 形态会被设备的安装门禁拦住**（这是设计如此）：native 包以设备用户直接
运行且不校验来源，所以 `deploy.native_apps` 为空时任何第三方 native 包都装不进去
（fail closed）。CLI 会先尝试向平台申请一份本地开发签名（`/api/v1/webide/dev-sign`，
经 daemon 用接入凭证转发），平台不支持时（例如线上平台返回 404）只剩一条路：
由设备管理员把 appid 加进 daemon 配置：

```yaml
# /etc/minictl.yaml —— 只写这一个键是安全的：
# parseConfig 以 DefaultConfig() 为源，只有文件里出现的键会覆盖默认值，
# 因此 mount/gateway/mdns/userdata 等行为与本文件不存在时完全一致。
deploy:
  native_apps:
    - com.nas.common.sdk-demo-native
```

改完需要 `minictl daemon restart`（会短暂重启全部 native 应用）。

**There is one more prerequisite on the SDK side**: `SYY_RUNTIME_SPEC` (RuntimeSpec v1) and
native contracts such as `system.storage.user-spaces`, together with the capability templates, are released with the daemon.
With an old daemon on the device, a native app shows up as "it installs and runs, but the RuntimeSpec is missing
(`app.FS()` returns `fs.ErrNoSpec`) and some capabilities report unknown template" — first upgrade the daemon
to a version that contains the corresponding changes.

---

## 12. 空间视图与元数据存储

这一节是"一个进程服务多个用户空间"的两个必备件：**按空间校验/寻址**，以及
元数据存储。

> **§12.2–12.4 是历史记录**：它们描述过渡期的"池内 SQLite"方案（历史包路径 `module/appsdk/db`，
> **已删除，没有迁进本仓库**）。
> 四个 FaaS 应用已回迁设备本地 PostgreSQL（每空间一个 schema，见 platform runtime 仓库的
> `docs/specs/faas-new-sdk-migration.md` §5），该 SQLite 层与 `modernc.org/sqlite`
> 依赖已从 SDK 删除。**新应用不要再按这一节选 SQLite**：要用就在自己的 go.mod 里
> 显式加驱动，别把 SQLite 重新变回 SDK 的默认依赖。

### 12.1 Space validation (`App.Space`)

native runs as a shared instance that "installs once and serves many spaces", so the space in a request must fall within
the set the instance is authorised for; otherwise data would be written to or read from **another user's** space.
The validation chain (in priority order):

```go
space, err := appsdk.SpaceFromHeaders(          // 1. identity headers -> space name (shape validation only)
    ctx.GetHeader(appconst.UsernameHeader),
    ctx.GetHeader(appconst.GroupNameHeader),
    ctx.GetHeader(appconst.NamespaceHeader),
)
if err != nil { /* 400: incomplete identity information */ }

view, err := app.Space(space)                    // 2. authorisation check + local view (no IO)
switch {
case errors.Is(err, appsdk.ErrSpaceNotAuthorized): // 403: space not in the authorised set
case errors.Is(err, appsdk.ErrInvalidSpace):       // 400: invalid space name (prevents directory escape)
case errors.Is(err, appsdk.ErrNoSpaceDataDir):     // 500: storage.app.file not declared
}

view.Name()            // alice-main
view.AppDataDir()      // <DataDir>/spaces/alice-main
view.Mounts()          // scope mounts of this space (scope/path/ro-rw)
view.Mount("music")    // core scopeKey (manifest space.music maps to core key "music")
view.Pool("syspool")   // usable when the pool is covered by exactly one scope; multiple scopes on the same pool return ErrAmbiguous
view.LocalPath("syspool", "photos/a.jpg")   // still goes through the fs escape checks
view.LocalPathForUFI(ufi)                    // resolved with the mount's space/pool/root
view.Ensure()          // creates AppDataDir (idempotent; only call it when about to write)
```

The criterion is the RuntimeSpec `spaces` (for multi-space native the daemon delivers them from `WebNamespaces`),
together with the union of `app.namespace` (container form) and `mounts[].space`;
`LocalPathForUFI` then maps the pool UFI back to a path relative to that scope's mount using
`mounts[].space + pool + root`.

Three points to remember:

1. **It only answers "can the data be touched"**. Authorisation for bus capabilities is still decided by the daemon alone (§4.1).
2. **No IO**. `App.Space` only reads the spec, so it can be called per request; directory creation is explicitly left to `Ensure()`,
   otherwise every request would need one more `mkdir`.
3. **Call `ReloadFS()` after the authorised spaces change**. After the AppStore fills in spaces for the logged-in user, the daemon
   regenerates the RuntimeSpec without restarting the process, but the SDK caches the spec/table. For example, when a request hits
   `ErrSpaceNotAuthorized`, calling `ReloadFS()` and retrying once is reasonable.

### 12.2 存储落点（为什么在 DataDir 下）

```text
<DataDir>/spaces/<space>/metadata.db
DataDir = MINI_APP_DATA_DIR = <poolRoot>/.app/storage/<app_id>   # 声明 storage.app.file
```

- **Into the pool**: DataDir sits in the storage pool chosen by the user, and `storage.app.file` creates the directory;
- **Kept on upgrade / cleared on uninstall**: the daemon purge cleans up exactly the **one** directory `<base>/.app/storage/<app_id>`.
  Putting the database in "each space's own pool" looks more natural, but it would not be cleaned up on uninstall and the data would
  remain on the user disk forever;
- multi-space isolation uses subdirectories, and space names go through the `ValidSpaceName` allow-list (letters, digits and
  `-_.` only; `..` and separators are forbidden), so even an odd space name in the spec cannot escape DataDir.
- new application-generated files (covers/cache/exports) go under `paths.data`; files scattered in the user pool
  `/.siyouyun/appdata/<app>/` from old environments no longer have a UFI resolution entry and need a one-off migration or rebuild by the app.

### 12.4 事件路径的端到端测试（`bustest`）

新 SDK 刻意不提供"可注入的总线"（`capability.New` 要真实 `*bus.Client`），应用因此**测不到
自己的事件处理路径**——只能直接调用处理函数，于是"订阅有没有接上、事件解码对不对、按空间
路由对不对"这一段永远空白，真机上的表现就是"代码看着对，事件就是不来"。

`bustest` 是一个最小假 daemon（WebSocket），实现了 subscribe/unsubscribe/call 应答与 event
推送，并按 daemon 的语义把 `system.interest.event` **展开**成 `system.interest.event.<AppID>`：

```go
srv := bustest.New(t, bustest.Options{AppID: "music", OnCall: fakeInterestListen})
client := bus.New(bus.Options{WSURL: srv.URL()})
listener, _ := capability.StartFileEventListener(ctx, client, capability.FileEventListenerOptions{
	Dirs: []string{"/Music"}, Transport: capability.TransportBus,
}, handler)
srv.WaitForSubscription(t, protocol.TopicInterestEvent)  // 订阅是异步的，先等它注册上
srv.Publish(t, protocol.TopicInterestEvent, interestEventPayload)  // 用生产者真实形状发事件
```

`fakeInterestListen` 按 `TemplateInterestListen` 返回 `{listenerId, transport:"bus", busTopic}`
即可；`bustest` 的 `OnCall` 是按模板定制应答的口子。

`WaitForSubscription` 不能省：紧接着 Publish 会偶发"事件先于订阅到达"，看起来像"事件没被
处理"——真机上这个竞态同样存在，只是被更长的启动时间掩盖了。

应用的事件处理路径现在可以端到端测：假 daemon → 真实 `bus.Client` → listener → handler →
队列/worker → 按事件 UGN 路由到该空间的库 → 落库，并断言幂等、越权空间被拒
（`ErrSpaceNotAuthorized`）、无关扩展名不入库。日志/统计（`srv.CallCount`、`srv.LastCall`、
`srv.Requests`）也在同一个假 daemon 上断言。

### 12.3 SQLite 元数据库（历史 `module/appsdk/db`）——**已删除，没有迁进本仓库；见上方说明**

```go
sp, err := app.Space(space)
if err != nil { /* 见 12.1 */ }
if err := sp.Ensure(); err != nil { /* 建目录 */ }

sdb, err := db.Open(ctx, db.Options{Path: db.PathIn(sp.AppDataDir())})
if err != nil { /* 见下面的失败语义 */ }
defer sdb.Close()

sdb.Write()   // 单连接、IMMEDIATE 事务：迁移与写入
sdb.Read()    // mode=ro + query_only：列表、搜索
```

设计要点（对应"读写分离 + WAL"）：

| 项 | 取值 | 原因 |
| --- | --- | --- |
| 驱动 | `modernc.org/sqlite`（纯 Go） | 应用交叉编译 `GOOS/GOARCH` 时 CGO 不可用 |
| 写连接池 | `MaxOpenConns(1)`，不设生命周期 | SQLite 只有一个写者；写连接还是 WAL `-shm` 的持有者 |
| 写事务 | `_txlock=immediate` | 进事务即拿写锁，避免读事务升级写事务时的 `BUSY_SNAPSHOT` |
| 读连接池 | `mode=ro` + `query_only`，多连接 | 读不被写阻塞（WAL 快照），且物理上不能写 |
| `journal_mode` | `WAL` | 强杀/掉电后自恢复且**不损坏**库文件；回读校验，不是 WAL 就明确失败 |
| `synchronous` | `NORMAL`（可换 `FULL`） | WAL 下 `NORMAL` 不会损坏，只可能丢最后一次提交 |
| `busy_timeout` | 5s | 瞬时争用先等，而不是把 `SQLITE_BUSY` 抛给业务 |
| `foreign_keys` | `1` | SQLite 默认关闭，gorm 模型带约束时不开等于没写 |
| 退出 | 先关读连接 → `wal_checkpoint(TRUNCATE)` → 关写连接 | 顺序不能反：checkpoint 需要没有别的读事务持快照 |
| 启动自检 | 库文件已存在时先 `PRAGMA quick_check` | **实测代价可忽略**：20 万行 / 63MB 的库只要 **44ms**（完整 `integrity_check` 311ms），所以默认每次启动都查——掉电、坏扇区造成的静默损坏能当场发现，而不是带着坏库继续写。库特别大且能接受风险时用 `Options.SkipIntegrityCheck` 跳过 |

两个必须遵守的不变量：

1. **写连接先打开并存活整个进程生命周期**。WAL 库的只读打开依赖 `-shm` 已存在，
   只读进程自己建不了它。`Open` 内部保证顺序，调用方不要给写连接设 `ConnMaxLifetime`。
2. **每个库只由本进程打开**，且必须落在本地文件系统上（SQLite 依赖 POSIX 文件锁，
   网络挂载不安全）。`Open` 会在库已存在时先做 `quick_check`，损坏时拒绝启动而不是
   带着坏库继续写。

接 gorm（gorm 是应用的业务选择，不进 SDK）：

```go
import (
    "gorm.io/driver/sqlite" // 只用它的 Dialector：真正干活的是 modernc
    "gorm.io/gorm"
)

w, err := gorm.Open(sqlite.Dialector{DriverName: db.DriverName, Conn: sdb.Write()}, &gorm.Config{})
r, err := gorm.Open(sqlite.Dialector{DriverName: db.DriverName, Conn: sdb.Read()}, &gorm.Config{})
```

三个容易踩的点：

1. **`Conn` 必须是已删除的 `db` 包（历史路径 `module/appsdk/db`）的句柄**，`DriverName` 用 `db.DriverName`（modernc 注册的
   `"sqlite"`）。`gorm.io/driver/sqlite` 只在 `Conn == nil` 时才 `sql.Open`，所以这里
   mattn 只是被 import、不会运行。
2. **`Conn` 不能省**。省掉就会走 `mattn/go-sqlite3`（CGO），在应用"从 macOS 交叉编译到
   Linux"的构建方式下没有 C 工具链，装不上也编不过。
   `gorm.io/driver/sqlite` 在 `CGO_ENABLED=0` 下能正常编译（mattn 有非 CGO stub），
   linux/amd64 与 linux/arm64 已实测通过。
   另外不要用 `sqlite.Open(dsn)`——那个会真的走 mattn。
3. 迁移（`gormigrate` / `AutoMigrate`）只跑在**写**句柄上；读句柄上的写会得到
   `attempt to write a readonly database`，这是刻意的（已实测）。

`db` 也导出纯字符串形式的 `db.WriteDSN(path)` / `db.ReadDSN(path)`，给不想用
`Conn:` 的场景；但用 `Open` 拿到的句柄更稳——DSN 与连接池配置只有一处。

---

## 13. 兴趣点监听（interest listener）

旧的总线文件事件通道（`system.file.event` + AppStore 中继 + `system.event.scope.get/set`
监听目录作用域）已经从 SDK v2 删除，替换为 **兴趣点监听（interest listener）**。
一个 listener 由 `dirs / mediaTypes / actions / excludeExts / qos / ttl` 描述，由 daemon 向
OS 申请；事件按监听条件在 OS 侧过滤。权限判据是 manifest 的 `event.file`。

### 13.1 两种 transport

| transport | 谁连 NATS | 事件如何到达应用 | ACK 与消费结果 |
| --- | --- | --- | --- |
| `nats`（默认） | 应用自己 | daemon 返回 OS 共享流 consumer + `nats_url`，应用自己 pull Fetch | 应用对共享流消息 ACK/NAK；消费结果回写 `os.interest.result`（JetStream 优先，短超时后回退 core NATS） |
| `bus`（纯总线） | daemon | daemon 侧 pump 拉 OS consumer，再扇出到 `system.interest.event.<app_id>`，应用只订阅总线 | 应用/SDK 调用 `system.interest.ack`；daemon 收到后才 ACK OS 消息，并在 success/failed 时回写 result |

两种模式相同的地方：

- daemon 通过 `system.interest.listen` 向 OS 申请 listener 并返回 `listenerId`；SDK 按 TTL
  周期 `system.interest.renew` 续租，续租失败会自动重新 listen（NATS 模式同时切换到新 consumer）。
- 事件体是 OS 兴趣点事件 `{v, eventId, ufi, srcUfi, action, inode, ugn, matched}`；SDK 解码成
  `capability.FileEvent`（补齐 `actionName` / `receivedAt`），按 `matched.listeners` 过滤，再交给 handler。
- `listener.Close()` 停止本地消费（bus 模式会退订总线主题），并调 `system.interest.stop` 释放
  daemon/OS 侧 listener。
- NATS 模式按 OS 下发的 worker 限制并发回调（`os.interest.control.*` 可动态调整）；bus 模式
  按事件到达顺序串行回调（daemon 已经接管 OS 侧消费，SDK 不再建 worker 池）。
- handler 返回 error 的语义取决于 transport：NATS 模式默认按 `retry` 上报并 NAK
  （at-least-once，`FailOnError:true` 时按终态 `failed` 上报并 ACK）；bus 模式必须调用 `system.interest.ack`（SDK 已封装），
  SDK 无法重投，handler 必须自行幂等/对账。

### 13.2 用法

```go
listener, err := capability.StartFileEventListener(ctx, app.Bus(), capability.FileEventListenerOptions{
	Dirs:        []string{"/Photos", "/Movies/2024"},
	MediaTypes:  []string{"image", "video"},
	Actions:     []string{"add", "delete", "rename"},
	ExcludeExts: []string{".tmp", ".part"},
	QoS:         "medium",
	Transport:   capability.TransportBus, // 省略或 TransportNATS = NATS 模式
}, func(ev capability.FileEvent) error {
	// ev.Space() / ev.Scope() 用来路由到正确的用户空间
	return nil
})
if err != nil {
	// daemon 不支持 event.file、transport 不合法、或 bus 模式未被 daemon 接受，都会在这里报错
	return err
}
defer listener.Close()

listener.ListenerID() // OS 颁发的监听 ID
listener.Transport()  // 生效的 transport
```

不支持的 `Transport` 取值会直接报错，**不会**静默退化到另一种 transport。

### 13.3 主机侧模板

| 模板 / 主题 | 用途 |
| --- | --- |
| `system.interest.listen` | 申请 listener；payload 带 `transport`（`nats`/`bus`）。nats 返回 consumer/stream/`natsUrl`，bus 返回 `busTopic` |
| `system.interest.stop` | 停止 listener 并释放 consumer/pump |
| `system.interest.renew` | 续租（`ttlSec`） |
| `system.interest.list` / `get` / `update` | 列出 / 查询 / 更新调用方自己的 listener（update 保持同一个 `listenerId`） |
| `system.interest.event`（topic） | 纯总线事件主题；订阅基础主题，daemon 展开成 `system.interest.event.<app_id>` |

常量见 `protocol.TemplateInterest*` 与 `protocol.TopicInterestEvent`。

### 13.4 从旧总线文件事件迁移

| 旧 API（已删除） | 现在 |
| --- | --- |
| `app.Events()` / `capability.Events` | `capability.StartFileEventListener` |
| `capability.SubscribeFileEvents(ctx, client, handler)` | `StartFileEventListener(..., TransportNATS, ...)`（需要 NATS） |
| `capability.SubscribeFileEventsReliable(...)` | 删除（总线可靠 ACK 不在 host 契约内）；改用 `StartFileEventListener(..., TransportNATS, ...)`，at-least-once 由 OS 共享流 consumer 提供 |
| `contrib/watchdirs` 监听目录作用域 | 删除；监听条件直接在 `FileEventListenerOptions` 里声明 |
| `system.file.event` / `system.event.scope.get/set` | `system.interest.event` / `system.interest.list/get/update` |

如果应用只需要"文件变化了通知我"、且运行环境不一定能直连 NATS（例如容器/受控网络），
用 `TransportBus`；需要自己控制消费节奏、重投语义或跨进程共享流 consumer 时用 `TransportNATS`。
