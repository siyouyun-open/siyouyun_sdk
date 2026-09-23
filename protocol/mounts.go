package protocol

// Mount table: the runtime tells the SDK which local directories the application can
// access directly.
//
// Why it is needed: a container and native get **completely different literal paths**:
//
//   - container: the pool is bound to <mount.container_mount_dest>/<export name>
//     (default /mnt/userfs/…);
//   - native: there is no mount namespace, so the pool is injected as a host absolute path.
//
// Yet both need the ability to "access files directly with POSIX" rather than through a
// network file interface. The SDK therefore unifies **logical addressing** (space + pool +
// relative path), while the literal path is resolved from the mount table the runtime
// provides. Export names are not fixed (for a regular application it is the pool directory
// name; at device level it is <user>_<ns>_<pool>), so they cannot be guessed or discovered
// by listing directories — the table is authoritative.
const (
	// MountTableVersion is the current mount table format version.
	MountTableVersion = 1
	// MountTableFileName is the mount table file name.
	MountTableFileName = "mounts.json"
	// ContainerResourceDir is the platform resource directory inside the container
	// (read-only bind).
	// native has no such directory and instead uses an environment variable pointing at
	// the mount table on the host.
	ContainerResourceDir = "/etc/miniruntime/resources"
	// EnvMountTable is the absolute path to the mount table (injected by the daemon for native).
	EnvMountTable = "MINI_APP_MOUNT_TABLE"
	// UFIPoolPrefix is the UFI pool prefix: /ufi/pool-raw/<pool>/<rel>.
	UFIPoolPrefix = "/ufi/pool-raw/"
)

// MountTable is the local resource view the runtime provides to the application.
type MountTable struct {
	Version int `json:"version"`
	// AppData is the application's private persistent directory (storage.app.file).
	AppData *MountEntry `json:"app_data,omitempty"`
	// Pools are the storage pools under the authorized space.
	Pools []MountEntry `json:"pools,omitempty"`
	// Scopes are the **data access scope values** declared by the application
	// (`space.music`, `space.music/2024`, `app`).
	//
	// Why they only matter for native: a container narrows access via bind mounts —
	// directories outside the scope are simply not mounted. A host process has no mount
	// namespace and the pool is still accessible as a whole directory, so "declared
	// space.music yet can read Documents" can only be blocked by the SDK itself from this
	// declaration (see the scope check in fs.Table).
	//
	// Empty means "no scope declaration" (old daemon / old package): no narrowing is
	// applied then, preserving existing behavior.
	Scopes []string `json:"scopes,omitempty"`
}

// MountEntry is a directory that can be accessed locally.
type MountEntry struct {
	// Name is the export name, and also the pool name in the UFI.
	// The application private directory has no export name on native, so it is empty there.
	Name string `json:"name,omitempty"`
	// Space is the user deployment space the pool belongs to (e.g. "alice-main", "group-team").
	//
	// A UFI carries no space information, and several user spaces may have pools with the
	// same name; Space lets the SDK address unambiguously, and also lets it map the UGN in
	// an event to the correct pool. A container instance naturally has only one space; a
	// shared native instance may have several.
	Space string `json:"space,omitempty"`
	// LocalPath is the absolute path the current process can open directly.
	LocalPath string `json:"local_path"`
	// Writable indicates whether the directory is writable (a deployment may be read-only
	// as a whole).
	Writable bool `json:"writable"`
	// Kind distinguishes the application private directory from a storage pool; an empty
	// value is equivalent to "pool".
	Kind string `json:"kind,omitempty"`
}

// Mount entry kinds.
const (
	MountKindPool    = "pool"
	MountKindAppData = "app_data"
)
