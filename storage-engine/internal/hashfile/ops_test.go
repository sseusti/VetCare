package hashfile

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"
)

func mustGet(t *testing.T, h *File, key []byte) (int64, bool) {
	t.Helper()
	v, ok, err := h.Get(key)
	if err != nil {
		t.Fatalf("Get(%x): %v", key, err)
	}
	return v, ok
}

func TestPutGetOverwriteDelete(t *testing.T) {
	h, _ := newTable(t, 4, 16)
	k := Int32Key(42)

	if _, ok := mustGet(t, h, k); ok {
		t.Fatal("empty table returned a key")
	}
	if err := h.Put(k, 100); err != nil {
		t.Fatal(err)
	}
	if v, ok := mustGet(t, h, k); !ok || v != 100 {
		t.Fatalf("Get = %d, %v; want 100, true", v, ok)
	}
	if err := h.Put(k, 200); err != nil {
		t.Fatal(err)
	}
	if v, _ := mustGet(t, h, k); v != 200 || h.Len() != 1 {
		t.Fatalf("after overwrite: value %d, len %d", v, h.Len())
	}
	if ok, err := h.Delete(k); err != nil || !ok {
		t.Fatalf("Delete = %v, %v", ok, err)
	}
	if _, ok := mustGet(t, h, k); ok || h.Len() != 0 {
		t.Fatal("key still present after Delete")
	}
	if ok, err := h.Delete(k); err != nil || ok {
		t.Fatalf("second Delete = %v, %v; want false", ok, err)
	}
}

func TestNegativeAndZeroValues(t *testing.T) {
	h, _ := newTable(t, 4, 16)
	h.Put(Int32Key(1), -1)
	h.Put(Int32Key(2), 0)
	if v, _ := mustGet(t, h, Int32Key(1)); v != -1 {
		t.Errorf("value -1 read back as %d", v)
	}
	if v, ok := mustGet(t, h, Int32Key(2)); !ok || v != 0 {
		t.Errorf("value 0 read back as %d, %v", v, ok)
	}
}

func TestStringKeys(t *testing.T) {
	h, _ := newTable(t, 32, 16)
	keys := map[string]int64{"Кошка": 1, "Собака": 2, "Попугай": 3, "a": 4}
	for k, v := range keys {
		if err := h.Put([]byte(k), v); err != nil {
			t.Fatal(err)
		}
	}
	for k, want := range keys {
		if v, ok := mustGet(t, h, []byte(k)); !ok || v != want {
			t.Errorf("Get(%q) = %d, %v; want %d", k, v, ok, want)
		}
	}
	if _, ok := mustGet(t, h, []byte("Кошк")); ok {
		t.Error("prefix of a key must not match")
	}
	long := []byte(strings.Repeat("я", 17)) // 34 bytes
	if err := h.Put(long, 1); !errors.Is(err, ErrKeySize) {
		t.Errorf("Put long key: want ErrKeySize, got %v", err)
	}
	if _, _, err := h.Get(long); !errors.Is(err, ErrKeySize) {
		t.Errorf("Get long key: want ErrKeySize, got %v", err)
	}
	if _, err := h.Delete(long); !errors.Is(err, ErrKeySize) {
		t.Errorf("Delete long key: want ErrKeySize, got %v", err)
	}
}

// collidingKeys returns n int32 keys that share one home slot in a table
// of the given capacity.
func collidingKeys(t *testing.T, capacity uint64, n int) [][]byte {
	t.Helper()
	target := hashKey(Int32Key(1)) & (capacity - 1)
	var keys [][]byte
	for i := int32(1); len(keys) < n; i++ {
		if k := Int32Key(i); hashKey(k)&(capacity-1) == target {
			keys = append(keys, k)
		}
		if i > 1_000_000 {
			t.Fatal("could not find colliding keys")
		}
	}
	return keys
}

func slotState(t *testing.T, h *File, i uint64) byte {
	t.Helper()
	s, _, _, err := h.readSlot(i)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCollisionsUseLinearProbing(t *testing.T) {
	h, _ := newTable(t, 4, 16)
	keys := collidingKeys(t, 16, 3)
	for i, k := range keys {
		if err := h.Put(k, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	home := hashKey(keys[0]) & 15
	for j := uint64(0); j < 3; j++ {
		if s := slotState(t, h, (home+j)&15); s != stateUsed {
			t.Fatalf("slot home+%d has state %d, want used", j, s)
		}
	}
	for i, k := range keys {
		if v, ok := mustGet(t, h, k); !ok || v != int64(i) {
			t.Errorf("key %d: Get = %d, %v", i, v, ok)
		}
	}
}

func TestTombstoneKeepsChainAndIsReused(t *testing.T) {
	h, _ := newTable(t, 4, 16)
	keys := collidingKeys(t, 16, 4)
	home := hashKey(keys[0]) & 15
	for i, k := range keys[:3] {
		h.Put(k, int64(i))
	}

	// Deleting the middle of the chain must leave a tombstone, otherwise
	// the third key would become unreachable.
	if ok, _ := h.Delete(keys[1]); !ok {
		t.Fatal("Delete middle key failed")
	}
	if s := slotState(t, h, (home+1)&15); s != stateDeleted {
		t.Fatalf("middle slot state %d, want tombstone", s)
	}
	if h.tombstones != 1 {
		t.Fatalf("tombstones = %d, want 1", h.tombstones)
	}
	if v, ok := mustGet(t, h, keys[2]); !ok || v != 2 {
		t.Fatalf("key behind tombstone: Get = %d, %v", v, ok)
	}

	// A new colliding key goes into the tombstone, not further down.
	if err := h.Put(keys[3], 3); err != nil {
		t.Fatal(err)
	}
	if h.tombstones != 0 {
		t.Fatalf("tombstone not reused: tombstones = %d", h.tombstones)
	}
	if v, ok := mustGet(t, h, keys[3]); !ok || v != 3 {
		t.Fatalf("reused slot: Get = %d, %v", v, ok)
	}
}

func TestDeleteAtChainEndLeavesNoTombstone(t *testing.T) {
	h, _ := newTable(t, 4, 16)
	keys := collidingKeys(t, 16, 2)
	h.Put(keys[0], 0)
	h.Put(keys[1], 1)

	h.Delete(keys[1]) // last in chain: next slot is empty
	if h.tombstones != 0 {
		t.Fatalf("tombstones = %d, want 0", h.tombstones)
	}
	if v, ok := mustGet(t, h, keys[0]); !ok || v != 0 {
		t.Fatalf("Get first key = %d, %v", v, ok)
	}
}

func TestDamagedSlotState(t *testing.T) {
	h, _ := newTable(t, 4, 16)
	k := Int32Key(7)
	h.writeSlot(hashKey(k)&15, 9, k, 0)
	if _, _, err := h.Get(k); !errors.Is(err, ErrBadFile) {
		t.Fatalf("want ErrBadFile, got %v", err)
	}
}

// TestRandomAgainstMap runs random operations against the table and a plain
// map in parallel and checks that they always agree, reopening the file
// from time to time.
func TestRandomAgainstMap(t *testing.T) {
	h, path := newTable(t, 4, 16)
	want := map[int32]int64{}
	rng := rand.New(rand.NewPCG(1, 2))

	for op := 0; op < 20_000; op++ {
		key := int32(rng.IntN(500))
		switch rng.IntN(3) {
		case 0, 1:
			val := rng.Int64()
			if err := h.Put(Int32Key(key), val); err != nil {
				t.Fatalf("op %d Put: %v", op, err)
			}
			want[key] = val
		case 2:
			ok, err := h.Delete(Int32Key(key))
			if err != nil {
				t.Fatalf("op %d Delete: %v", op, err)
			}
			if _, had := want[key]; ok != had {
				t.Fatalf("op %d Delete(%d) = %v, map had it: %v", op, key, ok, had)
			}
			delete(want, key)
		}
		if op%5000 == 4999 {
			h = reopen(t, h, path)
		}
	}

	if h.Len() != uint64(len(want)) {
		t.Fatalf("Len = %d, want %d", h.Len(), len(want))
	}
	for key := int32(0); key < 500; key++ {
		v, ok := mustGet(t, h, Int32Key(key))
		if wv, had := want[key]; ok != had || (ok && v != wv) {
			t.Fatalf("key %d: table (%d, %v), map (%d, %v)", key, v, ok, wv, had)
		}
	}
}
