package export

import (
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/sseusti/VetCare/storage-engine/internal/db"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

func sampleVisits() []record.Visit {
	return []record.Visit{
		{ID: 1, PetName: "Барсик", Species: "Кошка", Doctor: "Иванова А. П.",
			At: time.Date(2026, 11, 2, 14, 30, 0, 0, time.Local), Status: record.StatusVisited, Price: 1500.5},
		{ID: 2, PetName: "Шарик", Species: "Собака", Doctor: "Петров",
			At: time.Date(2026, 11, 3, 9, 0, 0, 0, time.Local), Status: record.StatusNoShow, Price: 0},
	}
}

func TestToXLSXFromDatabase(t *testing.T) {
	d, err := db.Create(filepath.Join(t.TempDir(), "vet"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, v := range sampleVisits() {
		if err := d.Insert(v); err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join(t.TempDir(), "visits.xlsx")
	n, err := ToXLSX(d, path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := f.GetRows(SheetName)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("sheet has %d rows, want header + 2", len(rows))
	}
	for i, h := range Header {
		if rows[0][i] != h {
			t.Errorf("header[%d] = %q, want %q", i, rows[0][i], h)
		}
	}

	byID := map[string][]string{}
	for _, r := range rows[1:] {
		byID[r[0]] = r
	}
	cat := byID["1"]
	if cat == nil {
		t.Fatal("visit 1 missing")
	}
	want := []string{"1", "Барсик", "Кошка", "Иванова А. П.", "02.11.2026 14:30", "Состоялся", "1,500.50"}
	for i := range want {
		if cat[i] != want[i] {
			t.Errorf("visit 1 column %d = %q, want %q", i, cat[i], want[i])
		}
	}
	if byID["2"][5] != "Не пришёл" {
		t.Errorf("status of visit 2 = %q", byID["2"][5])
	}

	// Price is stored as a number, not text, so Excel can sum it.
	raw, _ := f.GetCellValue(SheetName, cellOfRow(t, rows, "1", 7), excelize.Options{RawCellValue: true})
	if v, err := strconv.ParseFloat(raw, 64); err != nil || v != 1500.5 {
		t.Errorf("raw price = %q, want number 1500.5", raw)
	}
}

func cellOfRow(t *testing.T, rows [][]string, id string, col int) string {
	t.Helper()
	for i, r := range rows {
		if r[0] == id {
			c, err := excelize.CoordinatesToCellName(col, i+1)
			if err != nil {
				t.Fatal(err)
			}
			return c
		}
	}
	t.Fatalf("row %s not found", id)
	return ""
}

func TestToXLSXEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.xlsx")
	n, err := ToXLSX(sliceScanner(nil), path)
	if err != nil || n != 0 {
		t.Fatalf("ToXLSX = %d, %v", n, err)
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, _ := f.GetRows(SheetName)
	if len(rows) != 1 {
		t.Fatalf("empty export has %d rows, want only the header", len(rows))
	}
}

func TestToXLSXManyRows(t *testing.T) {
	var vs []record.Visit
	for i := int32(1); i <= 5000; i++ {
		v := sampleVisits()[0]
		v.ID = i
		vs = append(vs, v)
	}
	path := filepath.Join(t.TempDir(), "big.xlsx")
	if n, err := ToXLSX(sliceScanner(vs), path); err != nil || n != 5000 {
		t.Fatalf("ToXLSX = %d, %v", n, err)
	}
}

func TestToXLSXScanError(t *testing.T) {
	boom := errors.New("disk on fire")
	_, err := ToXLSX(failingScanner{boom}, filepath.Join(t.TempDir(), "x.xlsx"))
	if !errors.Is(err, boom) {
		t.Fatalf("want scan error, got %v", err)
	}
}

func TestToXLSXBadPath(t *testing.T) {
	_, err := ToXLSX(sliceScanner(sampleVisits()), filepath.Join(t.TempDir(), "no", "such", "dir", "x.xlsx"))
	if err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

type sliceScanner []record.Visit

func (s sliceScanner) Scan(fn func(record.Visit) bool) error {
	for _, v := range s {
		if !fn(v) {
			break
		}
	}
	return nil
}

type failingScanner struct{ err error }

func (f failingScanner) Scan(func(record.Visit) bool) error { return f.err }
