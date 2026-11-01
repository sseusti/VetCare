package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

func TestCreateMakesFourFiles(t *testing.T) {
	_, dir := newDB(t)
	for _, name := range Files {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCreateRefusesExistingDB(t *testing.T) {
	_, dir := newDB(t)
	if _, err := Create(dir); !errors.Is(err, ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
}

func TestOpenNotADatabase(t *testing.T) {
	if _, err := Open(t.TempDir()); !errors.Is(err, ErrNotDB) {
		t.Fatalf("empty dir: want ErrNotDB, got %v", err)
	}
	d, dir := newDB(t)
	d.Close()
	os.Remove(filepath.Join(dir, FileDoctor))
	if _, err := Open(dir); !errors.Is(err, ErrNotDB) {
		t.Fatalf("missing index: want ErrNotDB, got %v", err)
	}
}

func TestOpenDetectsInconsistentFiles(t *testing.T) {
	d, dir := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"))
	d.key.Delete(keyOf(1)) // index no longer matches the data
	d.Close()
	if _, err := Open(dir); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
}

func TestReopenKeepsEverything(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d,
		visit(1, "Кошка", "Иванова"),
		visit(2, "Собака", "Иванова"),
		visit(3, "Кошка", "Петров"),
	)
	d = reopen(t, d)
	if d.Len() != 3 {
		t.Fatalf("Len = %d, want 3", d.Len())
	}
	if v, err := d.Get(2); err != nil || v.Species != "Собака" {
		t.Fatalf("Get(2) = %+v, %v", v, err)
	}
	checkIntegrity(t, d)
}

func TestClear(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"), visit(2, "Собака", "Петров"))
	if err := d.Clear(); err != nil {
		t.Fatal(err)
	}
	if d.Len() != 0 {
		t.Fatalf("Len after Clear = %d", d.Len())
	}
	if found, _ := d.FindBy(FieldSpecies, "Кошка"); len(found) != 0 {
		t.Fatalf("FindBy after Clear = %v", found)
	}
	mustInsert(t, d, visit(1, "Кошка", "Иванова"))
	checkIntegrity(t, reopen(t, d))
}

func TestDropRemovesFiles(t *testing.T) {
	d, dir := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"))
	if err := d.Drop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory still exists: %v", err)
	}
}

func TestSyncAndScan(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"), visit(2, "Собака", "Петров"), visit(3, "Кошка", "Петров"))
	if err := d.Sync(); err != nil {
		t.Fatal(err)
	}
	var all []record.Visit
	d.Scan(func(v record.Visit) bool { all = append(all, v); return true })
	if !equalIDs(ids(all), []int32{1, 2, 3}) {
		t.Fatalf("Scan = %v", ids(all))
	}
	seen := 0
	d.Scan(func(record.Visit) bool { seen++; return false })
	if seen != 1 {
		t.Fatalf("Scan did not stop: %d", seen)
	}
}

func TestUseAfterClose(t *testing.T) {
	d, _ := newDB(t)
	d.Close()
	checks := map[string]error{
		"Insert":     d.Insert(visit(1, "Кошка", "Иванова")),
		"DeleteByID": d.DeleteByID(1),
		"Sync":       d.Sync(),
		"Clear":      d.Clear(),
		"Close":      d.Close(),
		"Scan":       d.Scan(func(record.Visit) bool { return true }),
		"Backup":     d.Backup(t.TempDir()),
		"Restore":    d.Restore(t.TempDir()),
	}
	_, getErr := d.Get(1)
	checks["Get"] = getErr
	_, findErr := d.FindBy(FieldSpecies, "Кошка")
	checks["FindBy"] = findErr
	for name, err := range checks {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("%s after Close: want ErrClosed, got %v", name, err)
		}
	}
	if d.Len() != 0 {
		t.Errorf("Len after Close = %d", d.Len())
	}
}
