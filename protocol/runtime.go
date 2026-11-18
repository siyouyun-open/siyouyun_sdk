package protocol

// ContainerRoot is the common parent of the AppStore runtime inside a container.
//
// Application-visible private data, user files, the RuntimeSpec, secrets, the bus
// and plugins all live under /lacx; /tmp outside /lacx is only used as a
// temporary cache.
const (
	ContainerRoot = "/lacx"

	// ContainerDataDir is the application's private persistent directory (RuntimeSpec paths.data).
	ContainerDataDir = ContainerRoot + "/data"
	// ContainerUserFSDir is the logical root of user-space authorisation (FileProvider/FUSE export).
	ContainerUserFSDir = ContainerRoot + "/userfs"
	// ContainerRuntimeDir is the read-only platform runtime resource directory.
	ContainerRuntimeDir = ContainerRoot + "/run"
	// ContainerSecretsDir is the reserved platform secrets directory (read-only).
	ContainerSecretsDir = ContainerRoot + "/secrets"
	// ContainerBusDir is the platform bus UDS directory.
	ContainerBusDir = ContainerRoot + "/bus"
	// ContainerPluginsDir is the dynamic plugin pool.
	ContainerPluginsDir = ContainerRoot + "/plugins"
	// ContainerResourceDir keeps the old name for compatibility; its value is unified to /lacx/run.
	ContainerResourceDir = ContainerRuntimeDir
	// UFIPoolPrefix is the UFI pool prefix: /ufi/pool-raw/<pool>/<rel>.
	UFIPoolPrefix = "/ufi/pool-raw/"

	// RuntimeSpecVersion is the only version number of RuntimeSpec v1.
	//
	// It must stay consistent with the const in docs/specs/runtime-spec-v1.schema.json
	// and with SDK v2 runtime.Version. The multi-space/UFI mapping fields are
	// additive extensions (no version bump):
	//
	//	spaces          []string   deployment spaces served by this instance (only spaces with core active scope grants)
	//	mounts[].space  string     deployment space this mount belongs to (native multi-space; may be empty in containers)
	//	mounts[].pool   string     storage pool name (e.g. syspool; empty for non-pool scopes)
	//	mounts[].root   string     pool-relative root of the scope (taken directly from the pool-internal
	//	                            prefix of the core grant UFI, e.g. Music/Photos; ""/" for the whole space; never guessed from the scope key)
	//	grants[].space  string     deployment space the grant belongs to (used together with scope to disambiguate multi-space)
	//
	// Note: mounts[].expiresAt / grants[].expiresAt are Unix **milliseconds** (the same unit as core grants);
	// 0 means no expiry (both the container-side scheduleGrantExpiry and native consume milliseconds).
	//
	// Older writers do not write these fields; readers must tolerate their absence (fall back to app.namespace / no pool information).
	RuntimeSpecVersion = 1
)
