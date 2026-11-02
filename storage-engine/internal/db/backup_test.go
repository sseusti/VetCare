package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupAndRestore(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"), visit(2, "Собака", "Петров"))

	bak := filepath.Join(t.TempDir(), "bak")
	if err := d.Backup(bak); err != nil {
		t.Fatal(err)
	}

	// Change the database after the backup.
	d.DeleteByID(1)
	mustInsert(t, d, visit(3, "Попугай", "Сидорова"))

	if err := d.Restore(bak); err != nil {
		t.Fatal(err)
	}
	if d.Len() != 2 {
		t.Fatalf("Len after restore = %d, want 2", d.Len())
	}
	if _, err := d.Get(1); err != nil {
		t.Fatalf("visit 1 not restored: %v", err)
	}
	if _, err := d.Get(3); !errors.Is(err, ErrNotFound) {
		t.Fatalf("visit 3 survived restore: %v", err)
	}
	checkIntegrity(t, d)

	// The database keeps working after a restore.
	mustInsert(t, d, visit(4, "Кошка", "Иванова"))
	checkIntegrity(t, reopen(t, d))
}

func TestBackupRefusesExistingBackup(t *testing.T) {
	d, _ := newDB(t)
	bak := filepath.Join(t.TempDir(), "bak")
	d.Backup(bak)
	if err := d.Backup(bak); !errors.Is(err, ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
}

func TestRestoreRejectsBadBackups(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"))

	if err := d.Restore(t.TempDir()); !errors.Is(err, ErrNotDB) {
		t.Errorf("empty dir: want ErrNotDB, got %v", err)
	}

	bak := filepath.Join(t.TempDir(), "bak")
	d.Backup(bak)
	os.WriteFile(filepath.Join(bak, FileSpecies), []byte("garbage"), 0o644)
	if err := d.Restore(bak); err == nil {
		t.Error("damaged backup was accepted")
	}

	// A rejected restore must leave the database as it was.
	if _, err := d.Get(1); err != nil {
		t.Fatalf("database damaged by rejected restore: %v", err)
	}
	checkIntegrity(t, d)
}

func TestRestoreRejectsInconsistentBackup(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"))
	bak := filepath.Join(t.TempDir(), "bak")
	d.Backup(bak)

	other, _ := newDB(t) // empty key index from another database
	other.Sync()
	data, _ := os.ReadFile(filepath.Join(other.dir, FileKey))
	os.WriteFile(filepath.Join(bak, FileKey), data, 0o644)

	if err := d.Restore(bak); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
}
