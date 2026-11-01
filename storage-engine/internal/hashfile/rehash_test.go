package hashfile

import (
	"os"
	"testing"
)

func TestRehashGrowsAndKeepsKeys(t *testing.T) {
	h, path := newTable(t, 4, 16)
	const n = 1000
	for i := int32(1); i <= n; i++ {
		if err := h.Put(Int32Key(i), int64(i)*10); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
		if h.count*maxLoadDen > h.capacity*maxLoadNum {
			t.Fatalf("load factor above 0.7 after %d inserts: %d/%d", i, h.count, h.capacity)
		}
	}
	if h.Capacity() != 2048 {
		t.Errorf("capacity = %d, want 2048 (1000 keys at load <= 0.7)", h.Capacity())
	}
	if _, err := os.Stat(path + ".rehash"); !os.IsNotExist(err) {
		t.Errorf("temporary rehash file left behind: %v", err)
	}

	h = reopen(t, h, path)
	if h.Len() != n || h.Capacity() != 2048 {
		t.Fatalf("after reopen: len %d, capacity %d", h.Len(), h.Capacity())
	}
	for i := int32(1); i <= n; i++ {
		if v, ok := mustGet(t, h, Int32Key(i)); !ok || v != int64(i)*10 {
			t.Fatalf("key %d after rehash: %d, %v", i, v, ok)
		}
	}
}

// Deleting from the middle of a probe chain leaves tombstones, and they
// count towards the load. When a rebuild is caused mostly by tombstones,
// it must drop them without growing the table.
func TestRehashDropsTombstonesWithoutGrowing(t *testing.T) {
	h, _ := newTable(t, 4, 16)
	keys := collidingKeys(t, 16, 12) // one chain of 11 slots + a spare key
	for i, k := range keys[:11] {
		if err := h.Put(k, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range keys[:6] { // each has a used slot after it
		h.Delete(k)
	}
	if h.count != 5 || h.tombstones != 6 {
		t.Fatalf("before rebuild: count %d, tombstones %d; want 5, 6", h.count, h.tombstones)
	}

	// (5 + 6 + 1) / 16 > 0.7 triggers a rebuild; 6 live keys <= half of 16,
	// so the size stays the same.
	if err := h.Put(keys[11], 11); err != nil {
		t.Fatal(err)
	}
	if h.Capacity() != 16 || h.tombstones != 0 || h.Len() != 6 {
		t.Fatalf("after rebuild: capacity %d, tombstones %d, len %d; want 16, 0, 6",
			h.Capacity(), h.tombstones, h.Len())
	}
	for i := 6; i < 12; i++ {
		if v, ok := mustGet(t, h, keys[i]); !ok || v != int64(i) {
			t.Fatalf("key %d after rebuild: %d, %v", i, v, ok)
		}
	}
	for _, k := range keys[:6] {
		if _, ok := mustGet(t, h, k); ok {
			t.Fatal("deleted key came back after rebuild")
		}
	}
}

func TestRehashFailsCleanlyWhenTempFileCannotBeCreated(t *testing.T) {
	h, path := newTable(t, 4, 16)
	// A directory in place of the temp file makes Create fail.
	if err := os.Mkdir(path+".rehash", 0o755); err != nil {
		t.Fatal(err)
	}
	var err error
	for i := int32(1); i <= 20 && err == nil; i++ {
		err = h.Put(Int32Key(i), 1)
	}
	if err == nil {
		t.Fatal("expected an error once a rehash was needed")
	}
	if v, ok := mustGet(t, h, Int32Key(1)); !ok || v != 1 {
		t.Fatalf("table damaged by failed rehash: %d, %v", v, ok)
	}
}
