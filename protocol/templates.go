package protocol

// Capability template constants.
//
// A template is the stable identifier of a capability: clients call by it, and the daemon
// routes by it. Writing them as constants rather than as scattered string literals makes
// typos surface at compile time instead of turning into a runtime "unknown template".
const (
	// ── KV storage (user dimension) ──
	// The caller does not pass username/namespace: for containers the daemon derives them
	// from the connection namespace; native is a shared instance, so they must be declared
	// explicitly and are validated against the instance's authorized space.
	TemplateKVGet    = "system.kv.get"
	TemplateKVPut    = "system.kv.put"
	TemplateKVList   = "system.kv.list"
	TemplateKVDelete = "system.kv.delete"

	// ── KV storage (application dimension) ──
	TemplateKVSysGet    = "system.kv.sys.get"
	TemplateKVSysPut    = "system.kv.sys.put"
	TemplateKVSysList   = "system.kv.sys.list"
	TemplateKVSysDelete = "system.kv.sys.delete"

	// ── Daemon and storage ──
	TemplateDaemonHealth     = "system.daemon.health"
	TemplateStorageSpaces    = "system.storage.spaces"
	TemplateStorageCheckPort = "system.storage.check-port"

	// ── Container / Compose management (used only by management applications) ──
	TemplateComposeDeploy       = "system.compose.deploy"
	TemplateComposeDeployStatus = "system.compose.deploy.status"
	TemplateComposeDeployCancel = "system.compose.deploy.cancel"
	TemplateComposeList         = "system.compose.list"
	TemplateComposeRemove       = "system.compose.remove"
	TemplateContainerList       = "system.container.list"
	TemplateContainerStart      = "system.container.start"
	TemplateContainerStop       = "system.container.stop"

	// ── 兴趣点监听/事件（daemon 申请 OS listener）──
	//
	// listen 支持两种 transport：
	//   - nats（默认）：返回 OS 共享流 consumer 与按调用方形态选择的 NATS 地址，
	//     应用自己连 NATS 做 pull Fetch；
	//   - bus：纯总线模式，daemon 侧 pump 连宿主机 NATS 拉取，再把原始事件扇出到
	//     system.interest.event.<app_id> 主题（应用不需要 NATS 地址）。
	//
	// list/get/update 只管理调用方自己的 listener：update 走 OS
	// PUT /os/interest/channel/{id}/filter，保持同一个 listenerId。
	TemplateInterestListen = "system.interest.listen"
	TemplateInterestStop   = "system.interest.stop"
	TemplateInterestRenew  = "system.interest.renew"
	TemplateInterestList   = "system.interest.list"
	TemplateInterestGet    = "system.interest.get"
	TemplateInterestUpdate = "system.interest.update"
	TemplateInterestAck    = "system.interest.ack"

	// ── Images (the schema is defined by the platform and implemented by a container provider) ──
	TemplateImageConvert   = "system.image.convert"
	TemplateImageThumbnail = "system.image.thumbnail"

	// ── Third-party route grants (gt-), provider = AppStore ──
	//
	// The provider is AppStore because it is the only party that knows which web route
	// belongs to which installed app: on an AppStore web route (/syy/faas/appstore/web/<hash>)
	// Core's self-binding makes appCode always "appstore", so Core cannot tell one caller
	// app from another. The grant's routePath is therefore derived by the provider from the
	// caller identity, and the caller must hand over the **user's own** platform token —
	// AppStore never signs with its own credentials (that would mint grants owned by the
	// wrong user). See docs/specs/route-grant-capability.md.
	//
	// Deliberately **not** exposed: update-expire. Core keeps /syy/oauth/thirdparty/update-expire;
	// no current caller needs it. To add it later, register a new template rather than
	// widening create.
	TemplateRouteGrantCreate = "system.route.grant.create"
	TemplateRouteGrantList   = "system.route.grant.list"
	TemplateRouteGrantToken  = "system.route.grant.token"
	TemplateRouteGrantRevoke = "system.route.grant.revoke"
	TemplateRouteGrantDelete = "system.route.grant.delete"
)

// Core / OS service proxy capabilities.
//
// These are interfaces to platform-owned services (Core :40100, OS :40000), proxied by the
// daemon with the caller identity injected: applications do not connect to Core/OS directly,
// so they neither assemble UGN headers themselves nor need to know the service addresses.
// User-dimension (NeedUGN) templates also require an explicit scope for native (see
// fillUGNForSubscriber).
//
// Deliberately **not** exposed:
//   - `system.app.info`: returns appAddr/appDSN/UGNList; the first is already injected via
//     environment variables, and the last is a tenant database credential, unsuitable for
//     delivery over the bus;
//   - `system.interest.event` in a call shape: file events are produced by the OS interest
//     listener and delivered as subscription events only (see TopicInterestEvent).
const (
	// ── Media ──
	TemplateMediaFFmpegInfo = "system.media.ffmpeg.info"

	// ── Devices ──
	TemplateDeviceServiceEndpoints = "system.device.service-endpoints"

	// ── User spaces (including group spaces and quotas) ──
	TemplateStorageUserSpaces = "system.storage.user-spaces"

	// ── Trash (POSIX cannot express Core's trash semantics, so it stays on the bus) ──
	TemplateFSTrash = "system.fs.trash"

	// ── Messages ──
	TemplateMessageSend = "system.message.send"

	// ── Scheduled tasks ──
	TemplateScheduleOnceCreate = "system.schedule.once.create"
	TemplateScheduleOnceUpdate = "system.schedule.once.update"
	TemplateScheduleCronCreate = "system.schedule.cron.create"
	TemplateScheduleCronUpdate = "system.schedule.cron.update"

	// ── Assets (models / runtimes) ──
	//
	// The daemon authenticates with the host enrollment (the app has no platform
	// identity) and hands the app short-lived presigned URLs; the app downloads,
	// resumes and verifies by itself so it owns its own progress.
	//
	// Note this deliberately hands a presigned URL to the caller — see
	// docs/specs/asset-api.md §6 for why that is allowed here (single-object,
	// read-only, short TTL) while the binding secret never leaves the daemon.
	TemplateAssetCatalog = "system.asset.catalog"
	TemplateAssetResolve = "system.asset.resolve"

	// ── AI gateway (daemon-provided; docs/spec/ai-gateway.md §8) ──
	//
	// `chat`/`models` are the daemon's front door for the AI gateway App; the gateway
	// itself is an ordinary App and the daemon never talks to an upstream provider.
	// `vault.lease` is the only plaintext-key path: the daemon proxies to vault with the
	// host internal token and takes app_id from the connection identity, never from the
	// payload. All three are **daemon-owned**: an app declaring them in its manifest is
	// rejected fail-closed (see registry-register.go and ValidateExposedFunction).
	TemplateAIChat     = "system.ai.chat"
	TemplateAIModels   = "system.ai.models"
	TemplateVaultLease = "system.vault.lease"
)

// Event topic constants.
const (
	// TopicInterestEvent is expanded by the daemon into system.interest.event.<app_id>
	// on subscription and is published by the daemon's pure-bus pump. It requires the
	// event.file permission (the daemon already filtered by the OS listener's dirs).
	TopicInterestEvent = "system.interest.event"
	// TopicScheduleEvent is the scheduled task event.
	TopicScheduleEvent = "system.event.schedule"
	// TopicNotificationEvent is the system notification event.
	TopicNotificationEvent = "system.event.notification"
)

// Mode is the mode of a capability.
type Mode string

const (
	// ModeCall is an RPC call.
	ModeCall Mode = "call"
	// ModeSubscribe is a topic subscription; such templates can only be subscribed to, not called.
	ModeSubscribe Mode = "subscribe"
)

// Capability is one piece of metadata in the capability catalog.
//
// The catalog lets clients (and code generators) know, **without connecting to a device**,
// which capabilities exist, which the daemon implements directly, and which need a provider
// App. The full JSON Schema is still authoritative from the daemon's `capability/list`; this
// only fixes identity and classification, and is kept aligned with the runtime registry by
// the daemon-side parity test.
type Capability struct {
	Template string
	Mode     Mode
	// DaemonProvided being true means the daemon executes it itself and no provider App is
	// needed; when it is false and Mode is call, an App providing the capability must exist.
	DaemonProvided bool
	// When AcceptFile is true the input contains a file, so it must go over the HTTP file
	// channel rather than a JSON frame.
	AcceptFile bool
	// Summary is a one-line description.
	Summary string
}

// Catalog returns the built-in capability catalog.
func Catalog() []Capability {
	return []Capability{
		{Template: TemplateKVGet, Mode: ModeCall, DaemonProvided: true, Summary: "Read user KV"},
		{Template: TemplateKVPut, Mode: ModeCall, DaemonProvided: true, Summary: "Write user KV"},
		{Template: TemplateKVList, Mode: ModeCall, DaemonProvided: true, Summary: "List user KV"},
		{Template: TemplateKVDelete, Mode: ModeCall, DaemonProvided: true, Summary: "Delete user KV"},
		{Template: TemplateKVSysGet, Mode: ModeCall, DaemonProvided: true, Summary: "Read app KV"},
		{Template: TemplateKVSysPut, Mode: ModeCall, DaemonProvided: true, Summary: "Write app KV"},
		{Template: TemplateKVSysList, Mode: ModeCall, DaemonProvided: true, Summary: "List app KV"},
		{Template: TemplateKVSysDelete, Mode: ModeCall, DaemonProvided: true, Summary: "Delete app KV"},

		{Template: TemplateDaemonHealth, Mode: ModeCall, DaemonProvided: true, Summary: "Daemon health check"},
		{Template: TemplateStorageSpaces, Mode: ModeCall, DaemonProvided: true, Summary: "List storage spaces"},
		{Template: TemplateStorageCheckPort, Mode: ModeCall, DaemonProvided: true, Summary: "Check host port availability"},

		{Template: TemplateComposeDeploy, Mode: ModeCall, DaemonProvided: true, Summary: "Deploy a Compose project asynchronously"},
		{Template: TemplateComposeDeployStatus, Mode: ModeCall, DaemonProvided: true, Summary: "Query deployment status"},
		{Template: TemplateComposeDeployCancel, Mode: ModeCall, DaemonProvided: true, Summary: "Cancel a deployment"},
		{Template: TemplateComposeList, Mode: ModeCall, DaemonProvided: true, Summary: "List Compose projects"},
		{Template: TemplateComposeRemove, Mode: ModeCall, DaemonProvided: true, Summary: "Remove a Compose project"},
		{Template: TemplateContainerList, Mode: ModeCall, DaemonProvided: true, Summary: "List managed containers"},
		{Template: TemplateContainerStart, Mode: ModeCall, DaemonProvided: true, Summary: "Start a container"},
		{Template: TemplateContainerStop, Mode: ModeCall, DaemonProvided: true, Summary: "Stop a container"},

		{Template: TemplateImageConvert, Mode: ModeCall, AcceptFile: true, Summary: "Convert an image format (requires a provider)"},
		{Template: TemplateImageThumbnail, Mode: ModeCall, AcceptFile: true, Summary: "Generate an image thumbnail (requires a provider)"},

		// Core / OS proxy: the daemon injects identity and forwards; applications do not
		// connect to platform services directly.
		{Template: TemplateMediaFFmpegInfo, Mode: ModeCall, DaemonProvided: true, Summary: "Read basic media file info"},
		{Template: TemplateDeviceServiceEndpoints, Mode: ModeCall, DaemonProvided: true, Summary: "Query device service endpoints"},
		{Template: TemplateStorageUserSpaces, Mode: ModeCall, DaemonProvided: true, Summary: "List user/group spaces and quotas"},
		{Template: TemplateFSTrash, Mode: ModeCall, DaemonProvided: true, Summary: "Move a file to the trash"},
		{Template: TemplateMessageSend, Mode: ModeCall, DaemonProvided: true, Summary: "Send a message as the app's bot"},
		{Template: TemplateScheduleOnceCreate, Mode: ModeCall, DaemonProvided: true, Summary: "Create a one-shot schedule"},
		{Template: TemplateScheduleOnceUpdate, Mode: ModeCall, DaemonProvided: true, Summary: "Update a one-shot schedule"},
		{Template: TemplateScheduleCronCreate, Mode: ModeCall, DaemonProvided: true, Summary: "Create a recurring schedule"},
		{Template: TemplateScheduleCronUpdate, Mode: ModeCall, DaemonProvided: true, Summary: "Update a recurring schedule"},

		// Assets (models / runtimes): the daemon authenticates with the host enrollment,
		// enforces the per-device allow list, and returns short-lived presigned URLs the
		// app downloads with itself. Bytes never travel over the bus.
		{Template: TemplateAssetCatalog, Mode: ModeCall, DaemonProvided: true, Summary: "List model/runtime assets available to this device"},
		{Template: TemplateAssetResolve, Mode: ModeCall, DaemonProvided: true, Summary: "Resolve an asset to short-lived presigned download URLs"},

		// AI gateway (docs/spec/ai-gateway.md §8). v1 is non-streaming: a 4k-token answer
		// fits in one 60 KiB frame, and the bus has no stream/cancel frames.
		{Template: TemplateAIChat, Mode: ModeCall, DaemonProvided: true, Summary: "Chat completion through the AI gateway (non-streaming)"},
		{Template: TemplateAIModels, Mode: ModeCall, DaemonProvided: true, Summary: "List the AI gateway model/alias catalog"},
		// Vault short-lived lease: the daemon proxies to vault with the host internal token;
		// the app allow list is fail-closed and app_id comes from the connection identity.
		{Template: TemplateVaultLease, Mode: ModeCall, DaemonProvided: true, Summary: "Lease a short-lived secret plaintext from vault (allow-listed apps only)"},

		// Third-party route grants: provider is AppStore (it owns the app→web-route mapping).
		// The caller supplies the user's platform token; the provider derives appCode/routePath
		// from the caller identity and refuses anything outside the caller's own routes.
		{Template: TemplateRouteGrantCreate, Mode: ModeCall, Summary: "Create a route grant (gt-) for the caller's own route prefix (requires the AppStore provider)"},
		{Template: TemplateRouteGrantList, Mode: ModeCall, Summary: "List route grants on the caller's own route prefixes (requires the AppStore provider)"},
		{Template: TemplateRouteGrantToken, Mode: ModeCall, Summary: "Reveal the plaintext token of one of the caller's own route grants (requires the AppStore provider)"},
		{Template: TemplateRouteGrantRevoke, Mode: ModeCall, Summary: "Revoke one of the caller's own route grants (keeps the record) (requires the AppStore provider)"},
		{Template: TemplateRouteGrantDelete, Mode: ModeCall, Summary: "Delete one of the caller's own route grants (record disappears) (requires the AppStore provider)"},

		{Template: TemplateInterestListen, Mode: ModeCall, DaemonProvided: true, Summary: "Create an interest listener (transport=nats returns an OS consumer + NATS URL; transport=bus returns a busTopic consumed by the daemon)"},
		{Template: TemplateInterestStop, Mode: ModeCall, DaemonProvided: true, Summary: "Stop an interest listener and release its consumer"},
		{Template: TemplateInterestRenew, Mode: ModeCall, DaemonProvided: true, Summary: "Renew an interest listener lease"},
		{Template: TemplateInterestList, Mode: ModeCall, DaemonProvided: true, Summary: "List the caller's own interest listeners"},
		{Template: TemplateInterestGet, Mode: ModeCall, DaemonProvided: true, Summary: "Get one of the caller's own interest listeners"},
		{Template: TemplateInterestUpdate, Mode: ModeCall, DaemonProvided: true, Summary: "Update dirs/actions/exclude/media of an existing interest listener (same listenerId)"},
		{Template: TemplateInterestAck, Mode: ModeCall, DaemonProvided: true, Summary: "Acknowledge or retry one pure-bus interest event"},
		{Template: TopicInterestEvent, Mode: ModeSubscribe, Summary: "Interest-point file events for the caller app (daemon pump, pure bus mode)"},
		{Template: TopicScheduleEvent, Mode: ModeSubscribe, Summary: "Schedule events"},
		{Template: TopicNotificationEvent, Mode: ModeSubscribe, Summary: "System notification events"},
	}
}

// Lookup looks up a template in the catalog.
func Lookup(template string) (Capability, bool) {
	for _, c := range Catalog() {
		if c.Template == template {
			return c, true
		}
	}
	return Capability{}, false
}
