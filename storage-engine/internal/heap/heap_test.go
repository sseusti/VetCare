package heap

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

func newHeap(t *testing.T) (*File, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "visits.dat")
	h, err := Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h, path
}

func reopen(t *testing.T, h *File, path string) *File {
	t.Helper()
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	h2, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { h2.Close() })
	return h2
}

func visit(id int32) record.Visit {
	return record.Visit{
		ID:      id,
		PetName: "Барсик",
		Species: "Кошка",
		Doctor:  "Иванова А. П.",
		At:      time.Unix(1_790_000_000+int64(id), 0),
		Status:  record.StatusScheduled,
		Price:   float64(id) * 100,
	}
}

// insert allocates a slot and writes a live record with the given id.
func insert(t *testing.T, h *File, id int32) int64 {
	t.Helper()
	off, err := h.Alloc()
	if err != nil {
		t.Fatalf("Alloc: %v", err)
	}
	if err := h.Write(off, record.Record{Live: true, Visit: visit(id), Links: record.NoLinks()}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return off
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Size()
}

func TestCreateEmpty(t *testing.T) {
	h, path := newHeap(t)
	if h.Len() != 0 || h.Slots() != 0 {
		t.Fatalf("Len %d, Slots %d; want 0, 0", h.Len(), h.Slots())
	}
	if size := fileSize(t, path); size != HeaderSize {
		t.Fatalf("file size %d, want %d", size, HeaderSize)
	}
}

func TestReopenKeepsRecordsAndFreeList(t *testing.T) {
	h, path := newHeap(t)
	a := insert(t, h, 1)
	b := insert(t, h, 2)
	insert(t, h, 3)
	h.Free(a)

	h = reopen(t, h, path)
	if h.Len() != 2 || h.Slots() != 3 {
		t.Fatalf("after reopen: Len %d, Slots %d; want 2, 3", h.Len(), h.Slots())
	}
	r, err := h.Read(b)
	if err != nil || !r.Live || r.Visit.ID != 2 {
		t.Fatalf("Read(b) after reopen = %+v, %v", r, err)
	}
	off, _ := h.Alloc()
	if off != a {
		t.Fatalf("free list lost on reopen: Alloc = %d, want %d", off, a)
	}
}

func TestOpenErrors(t *testing.T) {
	dir := t.TempDir()

	if _, err := Open(filepath.Join(dir, "missing.dat")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: want ErrNotExist, got %v", err)
	}

	short := filepath.Join(dir, "short.dat")
	os.WriteFile(short, []byte("VETHEAP1"), 0o644)
	if _, err := Open(short); !errors.Is(err, ErrBadFile) {
		t.Errorf("short file: want ErrBadFile, got %v", err)
	}

	garbage := filepath.Join(dir, "garbage.dat")
	os.WriteFile(garbage, make([]byte, HeaderSize), 0o644)
	if _, err := Open(garbage); !errors.Is(err, ErrBadFile) {
		t.Errorf("wrong magic: want ErrBadFile, got %v", err)
	}
}

func TestOpenBadHeaderFields(t *testing.T) {
	tests := []struct {
		name  string
		patch func(b []byte)
	}{
		{"other record size", func(b []byte) { binary.LittleEndian.PutUint32(b[8:], 100) }},
		{"more live than slots", func(b []byte) { binary.LittleEndian.PutUint64(b[20:], 99) }},
		{"slots do not match size", func(b []byte) { binary.LittleEndian.PutUint64(b[12:], 7) }},
		{"free head not a slot", func(b []byte) { binary.LittleEndian.PutUint64(b[28:], 70) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, path := newHeap(t)
			insert(t, h, 1)
			insert(t, h, 2)
			h.Close()
			b, _ := os.ReadFile(path)
			tc.patch(b)
			os.WriteFile(path, b, 0o644)
			if _, err := Open(path); !errors.Is(err, ErrBadFile) {
				t.Fatalf("want ErrBadFile, got %v", err)
			}
		})
	}
}

func TestClear(t *testing.T) {
	h, path := newHeap(t)
	a := insert(t, h, 1)
	insert(t, h, 2)
	h.Free(a)
	if err := h.Clear(); err != nil {
		t.Fatal(err)
	}
	if h.Len() != 0 || h.Slots() != 0 || fileSize(t, path) != HeaderSize {
		t.Fatalf("after Clear: Len %d, Slots %d, size %d", h.Len(), h.Slots(), fileSize(t, path))
	}
	if off := insert(t, h, 3); off != HeaderSize {
		t.Fatalf("first insert after Clear at %d, want %d", off, HeaderSize)
	}
	h = reopen(t, h, path)
	if h.Len() != 1 {
		t.Fatalf("Len after Clear and reopen = %d, want 1", h.Len())
	}
}

func TestUseAfterClose(t *testing.T) {
	h, _ := newHeap(t)
	off := insert(t, h, 1)
	h.Close()

	if _, err := h.Alloc(); !errors.Is(err, ErrClosed) {
		t.Errorf("Alloc: want ErrClosed, got %v", err)
	}
	if _, err := h.Read(off); !errors.Is(err, ErrClosed) {
		t.Errorf("Read: want ErrClosed, got %v", err)
	}
	if err := h.Free(off); !errors.Is(err, ErrClosed) {
		t.Errorf("Free: want ErrClosed, got %v", err)
	}
	if err := h.Scan(func(int64, record.Record) bool { return true }); !errors.Is(err, ErrClosed) {
		t.Errorf("Scan: want ErrClosed, got %v", err)
	}
	if err := h.Sync(); !errors.Is(err, ErrClosed) {
		t.Errorf("Sync: want ErrClosed, got %v", err)
	}
	if err := h.Clear(); !errors.Is(err, ErrClosed) {
		t.Errorf("Clear: want ErrClosed, got %v", err)
	}
	if err := h.Close(); !errors.Is(err, ErrClosed) {
		t.Errorf("second Close: want ErrClosed, got %v", err)
	}
}
