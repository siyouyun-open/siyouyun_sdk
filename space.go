package appsdk

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/siyouyun-open/siyouyun_sdk/v2/appconst"
	appfs "github.com/siyouyun-open/siyouyun_sdk/v2/fs"
)

// spacesDirName is the subdirectory name for each user space under the app's private directory.
//
//	<DataDir>/spaces/<space>/metadata.db
//
// Why it lives in the app private directory (storage.app.file) rather than the pool root:
// uninstall semantics. daemon's purge only cleans the single directory
// `<poolRoot>/.app/storage/<app_id>`; placed anywhere else (such as "a pool per space")
// it would stay on disk forever after uninstall. See docs/specs/faas-new-sdk-migration.md.
const spacesDirName = "spaces"

// Detectable errors related to spaces. Callers test them with errors.Is and map them to
// responses such as 403.
var (
	// ErrInvalidSpace indicates that the space name is invalid or the identity information is incomplete.
	ErrInvalidSpace = errors.New("appsdk: invalid space")
	// ErrSpaceNotAuthorized indicates that the space is not in the instance's authorized set.
	ErrSpaceNotAuthorized = errors.New("appsdk: space not authorized")
	// ErrNoSpaceDataDir indicates that the app did not declare storage.app.file, so per-space data cannot be stored.
	ErrNoSpaceDataDir = errors.New("appsdk: no app data directory (declare storage.app.file)")
)

// SpaceFromHeaders derives the deployment space of the current request from the identity
// headers injected by the platform.
//
// A personal space is {username}-{namespace} and a group space is {groupname}-{namespace};
// it shares fs.DeploySpace with daemon's rules, so that "the space used for validation" and
// "the space used to resolve paths" do not drift into two algorithms.
//
// It only validates the shape and **does not authorize** — authorization is App.Space's job
// (it needs the mount table).
func SpaceFromHeaders(username, groupName, namespace string) (string, error) {
	owner := strings.TrimSpace(username)
	if group := strings.TrimSpace(groupName); group != "" && group != owner {
		owner = group
	}
	if owner == "" {
		return "", fmt.Errorf("%w: identity headers are missing the user/group (%s / %s)", ErrInvalidSpace,
			appconst.UsernameHeader, appconst.GroupNameHeader)
	}
	if strings.TrimSpace(namespace) == "" {
		return "", fmt.Errorf("%w: identity headers are missing the space name (%s)", ErrInvalidSpace, appconst.NamespaceHeader)
	}
	return appfs.DeploySpace(username, groupName, namespace), nil
}

// ValidSpaceName validates the character set of a space name.
//
// A space name becomes a directory name, so this must be strict: only alphanumerics and
// `-_.` are allowed, and `..`, path separators, whitespace and control characters are
// rejected. Authorization (HasSpace) is the main gate; this allowlist is defense in depth —
// even if an odd space name appeared in the mount table, it could not be joined into a path
// that escapes DataDir.
func ValidSpaceName(space string) bool {
	if space == "" || len(space) > 128 {
		return false
	}
	if space == "." || space == ".." {
		return false
	}
	for _, r := range space {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

// SpaceView is the local view of "a user space" inside the app process: a private directory
// plus that space's pools.
//
// Obtaining one means the space has already passed authorization; it holds no open files or
// connections, so it can be acquired cheaply per request (App.Space only reads the mount
// table and performs no IO).
type SpaceView struct {
	name    string
	appData string
	table   *appfs.Table
	pools   []*appfs.Mount
}

// Name returns the deployment space identifier (such as alice-main / team-main).
func (v *SpaceView) Name() string { return v.name }

// AppDataDir returns the space's private directory (which may not exist yet; see Ensure).
func (v *SpaceView) AppDataDir() string { return v.appData }

// Pools returns the storage pools under this space.
func (v *SpaceView) Pools() []*appfs.Mount { return append([]*appfs.Mount(nil), v.pools...) }

// Pool resolves a pool by name within this space.
func (v *SpaceView) Pool(name string) (*appfs.Mount, error) {
	return v.table.PoolIn(v.name, name)
}

// LocalPath resolves (pool name, relative path) into an absolute local path within this space.
func (v *SpaceView) LocalPath(pool, rel string) (string, error) {
	mount, err := v.Pool(pool)
	if err != nil {
		return "", err
	}
	return mount.Resolve(rel)
}

// LocalPathForUFI resolves a UFI within this space.
func (v *SpaceView) LocalPathForUFI(ufi string) (string, error) {
	return v.table.LocalPathIn(v.name, ufi)
}

// Ensure creates the space's private directory (idempotent).
//
// Call it once at points that need to write to disk, such as before opening a database or
// writing a cache; read paths should not call it, otherwise every request pays for an extra
// mkdir syscall.
func (v *SpaceView) Ensure() error {
	if v.appData == "" {
		return fmt.Errorf("%w: %s", ErrNoSpaceDataDir, v.name)
	}
	if err := os.MkdirAll(v.appData, 0o750); err != nil {
		return fmt.Errorf("appsdk: failed to create space data directory %s: %w", v.appData, err)
	}
	return nil
}

// Space validates and returns the local view of a user space.
//
// Validation chain (the order is the priority):
//
//  1. the space name has a valid shape (prevents directory escape);
//  2. the space is in the mount table's authorized set (prevents cross-user privilege
//     escalation, "option A" of decision 4);
//  3. the app declared storage.app.file (otherwise there is no location to store per-space data).
//
// It performs no IO: directory creation is left to SpaceView.Ensure. After the authorized
// spaces change, the mount table must be ReloadFS'd before a new space becomes visible.
func (a *App) Space(name string) (*SpaceView, error) {
	name = strings.TrimSpace(name)
	if !ValidSpaceName(name) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidSpace, name)
	}
	table, err := a.FS()
	if err != nil {
		return nil, err
	}
	if !table.HasSpace(name) {
		return nil, fmt.Errorf("%w: %q (instance authorized spaces: %s)", ErrSpaceNotAuthorized, name,
			strings.Join(table.Spaces(), ", "))
	}
	dataDir := strings.TrimSpace(a.Runtime.DataDir)
	if dataDir == "" {
		return nil, fmt.Errorf("%w: space %s", ErrNoSpaceDataDir, name)
	}
	return &SpaceView{
		name:    name,
		appData: filepath.Join(dataDir, spacesDirName, name),
		table:   table,
		pools:   table.SpacePools(name),
	}, nil
}
