package hashfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// newTable creates a table in a temp dir and closes it at the end of the test.
func newTable(t *testing.T, keySize int, capacity uint64) (*File, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.idx")
	h, err := Create(path, keySize, capacity)
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

func TestCreateRoundsCapacity(t *testing.T) {
	tests := []struct{ in, want uint64 }{
		{0, 16}, {1, 16}, {16, 16}, {17, 32}, {1000, 1024},
	}
	for _, tc := range tests {
		h, _ := newTable(t, 4, tc.in)
		if h.Capacity() != tc.want {
			t.Errorf("Create(capacity %d).Capacity() = %d, want %d", tc.in, h.Capacity(), tc.want)
		}
	}
}

func TestCreateFileSize(t *testing.T) {
	h, path := newTable(t, 4, 16)
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(headerSize + 16*(1+4+8)); st.Size() != want {
		t.Errorf("file size = %d, want %d", st.Size(), want)
	}
	if h.Len() != 0 || h.KeySize() != 4 {
		t.Errorf("Len = %d, KeySize = %d", h.Len(), h.KeySize())
	}
}

func TestCreateBadKeySize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.idx")
	if _, err := Create(path, 0, 16); err == nil {
		t.Fatal("key size 0 must be rejected")
	}
}

func TestOpenKeepsHeader(t *testing.T) {
	h, path := newTable(t, 32, 64)
	if err := h.Put([]byte("Кошка"), 7); err != nil {
		t.Fatal(err)
	}
	h = reopen(t, h, path)
	if h.KeySize() != 32 || h.Capacity() != 64 || h.Len() != 1 {
		t.Fatalf("after reopen: keySize %d, capacity %d, len %d", h.KeySize(), h.Capacity(), h.Len())
	}
	if v, ok, err := h.Get([]byte("Кошка")); err != nil || !ok || v != 7 {
		t.Fatalf("Get after reopen = %d, %v, %v", v, ok, err)
	}
}

func TestOpenErrors(t *testing.T) {
	dir := t.TempDir()

	if _, err := Open(filepath.Join(dir, "missing.idx")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: want ErrNotExist, got %v", err)
	}

	short := filepath.Join(dir, "short.idx")
	os.WriteFile(short, []byte("VETHASH1"), 0o644)
	if _, err := Open(short); !errors.Is(err, ErrBadFile) {
		t.Errorf("short file: want ErrBadFile, got %v", err)
	}

	garbage := filepath.Join(dir, "garbage.idx")
	os.WriteFile(garbage, make([]byte, 200), 0o644)
	if _, err := Open(garbage); !errors.Is(err, ErrBadFile) {
		t.Errorf("wrong magic: want ErrBadFile, got %v", err)
	}

	h, path := newTable(t, 4, 16)
	h.Close()
	f, _ := os.OpenFile(path, os.O_RDWR, 0)
	f.Truncate(headerSize + 10)
	f.Close()
	if _, err := Open(path); !errors.Is(err, ErrBadFile) {
		t.Errorf("truncated file: want ErrBadFile, got %v", err)
	}
}

func TestOpenBadHeaderFields(t *testing.T) {
	tests := []struct {
		name  string
		patch func(b []byte)
	}{
		{"zero key size", func(b []byte) { b[8], b[9], b[10], b[11] = 0, 0, 0, 0 }},
		{"capacity not power of two", func(b []byte) { b[12] = 17 }},
		{"more keys than slots", func(b []byte) { b[20] = 200 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, path := newTable(t, 4, 16)
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
	h, path := newTable(t, 4, 16)
	for i := int32(1); i <= 100; i++ {
		if err := h.Put(Int32Key(i), int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Clear(); err != nil {
		t.Fatal(err)
	}
	if h.Len() != 0 || h.Capacity() != minCapacity {
		t.Fatalf("after Clear: len %d, capacity %d", h.Len(), h.Capacity())
	}
	if _, ok, _ := h.Get(Int32Key(5)); ok {
		t.Fatal("key survived Clear")
	}
	if err := h.Put(Int32Key(5), 50); err != nil {
		t.Fatal(err)
	}
	h = reopen(t, h, path)
	if v, ok, _ := h.Get(Int32Key(5)); !ok || v != 50 {
		t.Fatalf("Get after Clear and reopen = %d, %v", v, ok)
	}
}

func TestUseAfterClose(t *testing.T) {
	h, _ := newTable(t, 4, 16)
	h.Close()
	if err := h.Put(Int32Key(1), 1); !errors.Is(err, ErrClosed) {
		t.Errorf("Put: want ErrClosed, got %v", err)
	}
	if _, _, err := h.Get(Int32Key(1)); !errors.Is(err, ErrClosed) {
		t.Errorf("Get: want ErrClosed, got %v", err)
	}
	if _, err := h.Delete(Int32Key(1)); !errors.Is(err, ErrClosed) {
		t.Errorf("Delete: want ErrClosed, got %v", err)
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
