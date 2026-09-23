package fs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Resolve resolves a relative path inside the mount to a local absolute path. It operates on the
// **path itself** (the final component is not followed). Used by Lstat / Remove / Rename / Mkdir,
// which "operate on this entry".
//
// This is the only security boundary: a container has the mount namespace as a backstop, but a
// native process can write directly to the host. Therefore `..`, absolute paths and "directory
// symlinks pointing outside the mount" must all be blocked here, never left to the caller's
// discipline.
func (m *Mount) Resolve(rel string) (string, error) {
	return m.resolve(rel, false)
}

// resolveTarget is the same as Resolve, but follows the final component (when it is a symlink).
// Used by ReadFile / Open / Stat / WriteFile, which really do need to land on the target file.
func (m *Mount) resolveTarget(rel string) (string, error) {
	return m.resolve(rel, true)
}

func (m *Mount) resolve(rel string, followTarget bool) (string, error) {
	if m == nil || m.root == "" {
		return "", fmt.Errorf("%w: empty mount", ErrNoMount)
	}
	clean, err := cleanRelative(rel)
	if err != nil {
		return "", err
	}
	full := filepath.Join(m.root, clean)
	if clean == "." {
		return full, nil
	}
	root := m.rootForCheck()

	// Validate the **deepest existing ancestor directory**: a directory symlink escape must be
	// blocked here, otherwise "link-to-outside/x" or MkdirAll("link-to-outside/a/b") would bypass an
	// implementation that only checks the final component. We walk upward because the intermediate
	// directories to be created do not exist yet and EvalSymlinks would fail.
	//
	// When the final component is not followed (Remove/Rename/Lstat), what must be checked is its
	// **parent directory**: deleting/moving a symlink itself is a legitimate operation, and
	// resolving it would be misjudged as an escape.
	checkPath := full
	if !followTarget {
		checkPath = filepath.Dir(full)
	}
	if !within(root, evalDeepestAncestor(checkPath)) {
		return "", fmt.Errorf("%w: %q", ErrEscape, rel)
	}
	// When we really need to land on the target, follow the final component and re-check once more.
	if followTarget {
		if resolved, terr := filepath.EvalSymlinks(full); terr == nil && resolved != "" {
			if !within(root, resolved) {
				return "", fmt.Errorf("%w: %q", ErrEscape, rel)
			}
		}
	}
	return full, nil
}

// evalDeepestAncestor returns the real path of the deepest existing ancestor of path (with symlink
// resolution included). When path itself already exists, it returns path itself.
func evalDeepestAncestor(path string) string {
	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil && resolved != "" {
			return resolved
		}
		parent := filepath.Dir(current)
		if parent == current {
			return current
		}
		current = parent
	}
}

// rootForCheck returns the real root directory used for escape checks (the mount point itself may
// be a symlink).
func (m *Mount) rootForCheck() string {
	if m.realRoot != "" {
		return m.realRoot
	}
	if resolved, err := filepath.EvalSymlinks(m.root); err == nil && resolved != "" {
		return resolved
	}
	return m.root
}

// UFI returns the UFI of a relative path inside the mount.
func (m *Mount) UFI(rel string) string {
	if m == nil {
		return ""
	}
	clean, err := cleanRelative(rel)
	if err != nil || clean == "." {
		return UFI(m.name, "")
	}
	return UFI(m.name, clean)
}

// cleanRelative normalizes a relative path inside the mount.
// A leading "/" is accepted (common in UFIs), but any form that escapes the mount root is rejected.
func cleanRelative(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return ".", nil
	}
	if strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("%w: %q", ErrEscape, rel)
	}
	rel = strings.ReplaceAll(rel, `\`, "/")
	clean := path.Clean(strings.TrimLeft(rel, "/"))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q", ErrEscape, rel)
	}
	if clean == "" {
		return ".", nil
	}
	return clean, nil
}

// within reports whether target lies inside root.
func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// requireWritable performs no check before read operations; before write operations it reports a
// clear error up front instead of letting the caller receive an ambiguous EACCES/EROFS.
func (m *Mount) requireWritable() error {
	if !m.Writable() {
		return fmt.Errorf("%w: %s", ErrReadOnly, m.root)
	}
	return nil
}

// ── read operations ──

// Stat returns path information (follows symlinks).
func (m *Mount) Stat(rel string) (os.FileInfo, error) {
	full, err := m.resolveTarget(rel)
	if err != nil {
		return nil, err
	}
	return os.Stat(full)
}

// Lstat returns path information (does not follow symlinks).
func (m *Mount) Lstat(rel string) (os.FileInfo, error) {
	full, err := m.Resolve(rel)
	if err != nil {
		return nil, err
	}
	return os.Lstat(full)
}

// ReadDir reads directory entries. An empty rel means the mount root.
func (m *Mount) ReadDir(rel string) ([]os.DirEntry, error) {
	full, err := m.resolveTarget(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(full)
}

// Open opens a file read-only.
func (m *Mount) Open(rel string) (*os.File, error) {
	full, err := m.resolveTarget(rel)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

// ReadFile reads the entire file.
func (m *Mount) ReadFile(rel string) ([]byte, error) {
	full, err := m.resolveTarget(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(full)
}

// Exists reports whether the path exists (distinguishing "not found" from other errors).
func (m *Mount) Exists(rel string) (bool, error) {
	full, err := m.Resolve(rel)
	if err != nil {
		return false, err
	}
	if _, err := os.Lstat(full); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Walk walks the subtree under the mount root.
func (m *Mount) Walk(rel string, fn filepath.WalkFunc) error {
	full, err := m.resolveTarget(rel)
	if err != nil {
		return err
	}
	return filepath.Walk(full, fn)
}

// ── write operations ──

// OpenFile opens a file. Flags with write intent fail early on a read-only mount.
func (m *Mount) OpenFile(rel string, flag int, perm os.FileMode) (*os.File, error) {
	if isWriteFlag(flag) {
		if err := m.requireWritable(); err != nil {
			return nil, err
		}
	}
	full, err := m.resolveTarget(rel)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(full, flag, perm)
}

// Create creates (or truncates) a file.
func (m *Mount) Create(rel string) (*os.File, error) {
	return m.OpenFile(rel, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
}

// WriteFile writes the entire file.
func (m *Mount) WriteFile(rel string, data []byte, perm os.FileMode) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	full, err := m.resolveTarget(rel)
	if err != nil {
		return err
	}
	return os.WriteFile(full, data, perm)
}

// Mkdir creates a single directory.
func (m *Mount) Mkdir(rel string, perm os.FileMode) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	full, err := m.Resolve(rel)
	if err != nil {
		return err
	}
	return os.Mkdir(full, perm)
}

// MkdirAll creates directories recursively.
func (m *Mount) MkdirAll(rel string, perm os.FileMode) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	full, err := m.Resolve(rel)
	if err != nil {
		return err
	}
	return os.MkdirAll(full, perm)
}

// Remove removes a file or an empty directory.
func (m *Mount) Remove(rel string) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	full, err := m.Resolve(rel)
	if err != nil {
		return err
	}
	return os.Remove(full)
}

// RemoveAll removes recursively.
func (m *Mount) RemoveAll(rel string) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	full, err := m.Resolve(rel)
	if err != nil {
		return err
	}
	return os.RemoveAll(full)
}

// Truncate truncates a file.
func (m *Mount) Truncate(rel string, size int64) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	full, err := m.resolveTarget(rel)
	if err != nil {
		return err
	}
	return os.Truncate(full, size)
}

// Chtimes changes the access/modification times.
func (m *Mount) Chtimes(rel string, atime, mtime time.Time) error {
	if err := m.requireWritable(); err != nil {
		return err
	}
	full, err := m.resolveTarget(rel)
	if err != nil {
		return err
	}
	return os.Chtimes(full, atime, mtime)
}

// isWriteFlag reports whether an open flag carries write intent.
func isWriteFlag(flag int) bool {
	const writeBits = os.O_WRONLY | os.O_RDWR | os.O_CREATE | os.O_TRUNC | os.O_APPEND
	return flag&writeBits != 0
}
