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

	// ── Images (the schema is defined by the platform and implemented by a container provider) ──
	TemplateImageConvert   = "system.image.convert"
	TemplateImageThumbnail = "system.image.thumbnail"

	// ── Qdrant vector database (implemented by the Qdrant container) ──
	TemplateQdrantHealth           = "infra.qdrant.health"
	TemplateQdrantCollectionsList  = "infra.qdrant.collections.list"
	TemplateQdrantCollectionInfo   = "infra.qdrant.collection.info"
	TemplateQdrantCollectionCreate = "infra.qdrant.collection.create"
	TemplateQdrantCollectionDelete = "infra.qdrant.collection.delete"
	TemplateQdrantPointsUpsert     = "infra.qdrant.points.upsert"
	TemplateQdrantPointsSetPayload = "infra.qdrant.points.set_payload"
	TemplateQdrantPointsDelete     = "infra.qdrant.points.delete"
	TemplateQdrantPointsQuery      = "infra.qdrant.points.query"
	TemplateQdrantPointsScroll     = "infra.qdrant.points.scroll"

	// ── File event watch scope (provider = AppStore) ──
	// Applications read and write their own watch directories through the SDK's
	// app.Events(); the value syntax is the same grammar ParseScopeValue implements.
	TemplateEventScopeGet = "system.event.scope.get"
	TemplateEventScopeSet = "system.event.scope.set"
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
//   - `system.file.event.config.*` / `.register`: file events on the minictl path are
//     produced and relayed uniformly by AppStore's global watcher, so applications do not
//     need to register watched directories one by one;
//   - `system.file.event.exists`: with a local mount table, applications can just stat the
//     local path.
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
)

// Event topic constants.
const (
	// TopicFileEvent is expanded by the daemon into system.file.event.<app_id> on
	// subscription; it requires storage.user.file (this authorized space) or
	// storage.device.file (device level).
	TopicFileEvent = "system.file.event"
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

		{Template: TemplateQdrantHealth, Mode: ModeCall, Summary: "Qdrant health check (requires a provider)"},
		{Template: TemplateQdrantCollectionsList, Mode: ModeCall, Summary: "List collections"},
		{Template: TemplateQdrantCollectionInfo, Mode: ModeCall, Summary: "Read collection info"},
		{Template: TemplateQdrantCollectionCreate, Mode: ModeCall, Summary: "Create a collection"},
		{Template: TemplateQdrantCollectionDelete, Mode: ModeCall, Summary: "Delete a collection"},
		{Template: TemplateQdrantPointsUpsert, Mode: ModeCall, Summary: "Upsert points"},
		{Template: TemplateQdrantPointsSetPayload, Mode: ModeCall, Summary: "Set the points payload"},
		{Template: TemplateQdrantPointsDelete, Mode: ModeCall, Summary: "Delete points"},
		{Template: TemplateQdrantPointsQuery, Mode: ModeCall, Summary: "Query vectors"},
		{Template: TemplateQdrantPointsScroll, Mode: ModeCall, Summary: "Scroll points"},

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

		// File event watch scope (design doc v4 §4.1): **not** daemon-provided; the provider is
		// AppStore (it holds the persistent configuration; the daemon only filters by scope).
		// The SDK's app.Events() is a thin wrapper over these two templates, so the template
		// names must be in the SDK's compile-time contract.
		{Template: TemplateEventScopeGet, Mode: ModeCall, Summary: "Read the app's file-event watch scope (requires the AppStore provider)"},
		{Template: TemplateEventScopeSet, Mode: ModeCall, Summary: "Change the app's file-event watch scope (requires the AppStore provider)"},

		{Template: TopicFileEvent, Mode: ModeSubscribe, Summary: "File changes under the app data directory"},
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
