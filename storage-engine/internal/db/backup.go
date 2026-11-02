package db

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sseusti/VetCare/storage-engine/internal/hashfile"
	"github.com/sseusti/VetCare/storage-engine/internal/heap"
)

// Backup copies all database files into dest, creating it if needed. dest
// must not already hold a database.
func (d *DB) Backup(dest string) error {
	if err := d.Sync(); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dest, FileData)); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, dest)
	}
	for _, name := range Files {
		if err := copyFile(d.path(name), filepath.Join(dest, name)); err != nil {
			return err
		}
	}
	return nil
}

// Restore replaces the database with the backup in src.
//
// The backup is checked first by opening it. Its files are then copied next
// to the current ones and renamed over them, so a failed copy leaves the
// current database untouched.
func (d *DB) Restore(src string) error {
	if d.heap == nil {
		return ErrClosed
	}
	if err := checkBackup(src); err != nil {
		return err
	}
	for _, name := range Files {
		if err := copyFile(filepath.Join(src, name), d.path(name)+".restore"); err != nil {
			removeRestoreFiles(d)
			return err
		}
	}
	d.closeFiles()
	for _, name := range Files {
		if err := os.Rename(d.path(name)+".restore", d.path(name)); err != nil {
			return err
		}
	}
	return d.openFiles()
}

// checkBackup opens every file of a backup to make sure it is valid.
func checkBackup(src string) error {
	h, err := heap.Open(filepath.Join(src, FileData))
	if err != nil {
		return backupErr(src, err)
	}
	live := h.Len()
	h.Close()
	for i, name := range Files[1:] {
		f, err := hashfile.Open(filepath.Join(src, name))
		if err != nil {
			return backupErr(src, err)
		}
		if i == 0 && f.Len() != live {
			f.Close()
			return fmt.Errorf("%w: backup %s has %d records and %d keys", ErrCorrupt, src, live, f.Len())
		}
		f.Close()
	}
	return nil
}

func backupErr(src string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s: %v", ErrNotDB, src, err)
	}
	return fmt.Errorf("backup %s: %w", src, err)
}

func removeRestoreFiles(d *DB) {
	for _, name := range Files {
		os.Remove(d.path(name) + ".restore")
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
