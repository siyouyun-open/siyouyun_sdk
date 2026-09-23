package fs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// renameFn lets tests inject a cross-device failure: a real EXDEV requires two different
// filesystems, which cannot be constructed portably in a unit test.
var renameFn = os.Rename

// Rename renames/moves within the same mount.
func (m *Mount) Rename(oldRel, newRel string) error {
	return Move(m, oldRel, m, newRel)
}

// Move moves a file or directory between two (possibly different) mounts.
//
// Why os.Rename alone is not enough: each export inside the container is an independent bind
// mount (an independent filesystem), so a cross-pool move inevitably returns EXDEV; native
// pools may also land on different datasets. Here it uniformly falls back to "recursive copy
// + delete the source", so callers see semantics consistent with the local filesystem.
func Move(src *Mount, srcRel string, dst *Mount, dstRel string) error {
	if src == nil || dst == nil {
		return fmt.Errorf("%w: nil mount", ErrNoMount)
	}
	if err := src.requireWritable(); err != nil {
		return err
	}
	if err := dst.requireWritable(); err != nil {
		return err
	}
	srcFull, err := src.Resolve(srcRel)
	if err != nil {
		return err
	}
	dstFull, err := dst.Resolve(dstRel)
	if err != nil {
		return err
	}
	if srcFull == dstFull {
		return nil
	}

	err = renameFn(srcFull, dstFull)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	if copyErr := copyTree(srcFull, dstFull); copyErr != nil {
		return fmt.Errorf("fs: cross-device move %s -> %s: %w", srcFull, dstFull, copyErr)
	}
	if rmErr := os.RemoveAll(srcFull); rmErr != nil {
		// The destination was already written successfully; since the source cannot be removed, the only option is to return an error so the caller knows a copy exists.
		return fmt.Errorf("fs: cross-device move removed %s but failed to clean %s: %w", dstFull, srcFull, rmErr)
	}
	return nil
}

// copyTree recursively copies regular files, directories, and symlinks; any other type is reported explicitly rather than silently skipped.
func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)

	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyTree(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil

	case info.Mode().IsRegular():
		return copyFile(src, dst, info.Mode().Perm())

	default:
		return fmt.Errorf("unsupported file type %s for %s", info.Mode(), src)
	}
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	// Flush to disk before deleting the source: a cross-device move most fears "the source was deleted but the destination is still in the page cache".
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
