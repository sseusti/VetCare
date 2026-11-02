package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/sseusti/VetCare/storage-engine/internal/db"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

func newApp(t *testing.T) *App {
	t.Helper()
	u := New(test.NewTempApp(t))
	t.Cleanup(func() {
		if u.db != nil {
			u.db.Close()
		}
	})
	return u
}

func visit(id int32, species, doctor string) record.Visit {
	return record.Visit{
		ID: id, PetName: "Барсик", Species: species, Doctor: doctor,
		At: time.Date(2026, 11, 2, 10, 0, 0, 0, time.Local), Status: record.StatusScheduled, Price: 1000,
	}
}

// withData creates a database with five visits and opens it in the app.
func withData(t *testing.T) *App {
	t.Helper()
	u := newApp(t)
	u.createDB(filepath.Join(t.TempDir(), "vet"))
	if u.db == nil {
		t.Fatal("database was not created")
	}
	for _, v := range []record.Visit{
		visit(1, "Кошка", "Иванова"),
		visit(2, "Собака", "Иванова"),
		visit(3, "Кошка", "Петров"),
		visit(4, "Попугай", "Петров"),
		visit(5, "Кошка", "Сидорова"),
	} {
		if err := u.insert(v); err != nil {
			t.Fatal(err)
		}
	}
	return u
}

func TestStartsWithoutDatabase(t *testing.T) {
	u := newApp(t)
	if !strings.Contains(u.win.Title(), "не открыта") {
		t.Errorf("title = %q", u.win.Title())
	}
	if u.needDB() {
		t.Error("needDB must be false without a database")
	}
}

func TestCreateInsertAndReopen(t *testing.T) {
	u := withData(t)
	if len(u.rows) != 5 || u.db.Len() != 5 {
		t.Fatalf("rows %d, Len %d; want 5", len(u.rows), u.db.Len())
	}
	if !strings.Contains(u.status.Text, "Записей в базе: 5") {
		t.Errorf("status = %q", u.status.Text)
	}
	dir := u.db.Dir()
	if !strings.Contains(u.win.Title(), dir) {
		t.Errorf("title %q does not name %s", u.win.Title(), dir)
	}

	u.onCloseDB()
	if u.db != nil || len(u.rows) != 0 {
		t.Fatal("close did not reset the state")
	}
	u.openDB(dir)
	if u.db == nil || len(u.rows) != 5 {
		t.Fatal("reopen lost data")
	}
}

func TestInsertDuplicateKeepsTable(t *testing.T) {
	u := withData(t)
	if err := u.insert(visit(3, "Хомяк", "Кузнецов")); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	if len(u.rows) != 5 {
		t.Fatalf("rows = %d after failed insert", len(u.rows))
	}
}

func TestSearchAndReset(t *testing.T) {
	u := withData(t)
	u.search(db.FieldSpecies, "Кошка")
	if len(u.rows) != 3 || !u.filtered {
		t.Fatalf("search rows = %d, filtered %v", len(u.rows), u.filtered)
	}
	if !strings.Contains(u.status.Text, "вторичный индекс") {
		t.Errorf("status does not name the index: %q", u.status.Text)
	}

	// A change while a search is shown keeps the search.
	if err := u.insert(visit(6, "Кошка", "Петров")); err != nil {
		t.Fatal(err)
	}
	if len(u.rows) != 4 || !u.filtered {
		t.Fatalf("after insert during search: rows %d, filtered %v", len(u.rows), u.filtered)
	}

	u.search(db.FieldPetName, "Барсик")
	if !strings.Contains(u.status.Text, "полный проход") {
		t.Errorf("status = %q", u.status.Text)
	}
	u.onReset()
	if len(u.rows) != 6 || u.filtered {
		t.Fatalf("reset: rows %d, filtered %v", len(u.rows), u.filtered)
	}
}

func TestUpdateAndDeleteSelected(t *testing.T) {
	u := withData(t)
	nv := visit(2, "Собака", "Петров")
	nv.PetName = "Шарик"
	if err := u.update(2, nv); err != nil {
		t.Fatal(err)
	}
	if v, _ := u.db.Get(2); v.PetName != "Шарик" || v.Doctor != "Петров" {
		t.Fatalf("update not saved: %+v", v)
	}

	u.selected = 0
	v, ok := u.selectedVisit()
	if !ok {
		t.Fatal("no selected visit")
	}
	u.deleteByID(v.ID)
	if u.db.Len() != 4 || len(u.rows) != 4 {
		t.Fatalf("after delete: Len %d, rows %d", u.db.Len(), len(u.rows))
	}
	if u.selected != -1 {
		t.Error("selection not reset after reload")
	}
}

func TestDeleteBy(t *testing.T) {
	u := withData(t)
	u.deleteBy(db.FieldDoctor, "Петров")
	if u.db.Len() != 3 {
		t.Fatalf("Len = %d, want 3", u.db.Len())
	}
	if !strings.Contains(u.status.Text, "Удалено приёмов: 2") {
		t.Errorf("status = %q", u.status.Text)
	}
}

func TestBackupRestore(t *testing.T) {
	u := withData(t)
	dest, err := u.backup(time.Date(2026, 11, 2, 15, 30, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dest, "vet-backup-20261102-153000") {
		t.Errorf("backup dir = %s", dest)
	}
	u.clearDB()
	if u.db.Len() != 0 || len(u.rows) != 0 {
		t.Fatal("clear did not empty the table")
	}
	u.restore(dest)
	if u.db.Len() != 5 || len(u.rows) != 5 {
		t.Fatalf("restore: Len %d, rows %d", u.db.Len(), len(u.rows))
	}
}

func TestExportAddsExtension(t *testing.T) {
	u := withData(t)
	base := filepath.Join(t.TempDir(), "visits")
	u.exportTo(base)
	if _, err := os.Stat(base + ".xlsx"); err != nil {
		t.Fatalf("export file: %v", err)
	}
	if !strings.Contains(u.status.Text, "Экспортировано строк: 5") {
		t.Errorf("status = %q", u.status.Text)
	}
}

func TestDropDB(t *testing.T) {
	u := withData(t)
	dir := u.db.Dir()
	u.dropDB()
	if u.db != nil {
		t.Fatal("db still set after drop")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("database folder still exists: %v", err)
	}
}

func TestTableCells(t *testing.T) {
	u := withData(t)
	rows, cols := u.table.Length()
	if rows != 5 || cols != len(columns) {
		t.Fatalf("table size %dx%d", rows, cols)
	}
	cells := rowCells(visit(7, "Кошка", "Иванова"))
	want := []string{"7", "Барсик", "Кошка", "Иванова", "02.11.2026 10:00", "Запланирован", "1000.00"}
	for i := range want {
		if cells[i] != want[i] {
			t.Errorf("cell %d = %q, want %q", i, cells[i], want[i])
		}
	}
}
