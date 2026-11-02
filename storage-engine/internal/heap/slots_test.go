package heap

import (
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

func TestAllocAppendsSlots(t *testing.T) {
	h, path := newHeap(t)
	for i := int32(0); i < 3; i++ {
		off := insert(t, h, i+1)
		if want := HeaderSize + int64(i)*record.RecordSize; off != want {
			t.Fatalf("slot %d at offset %d, want %d", i, off, want)
		}
	}
	if h.Len() != 3 || h.Slots() != 3 {
		t.Fatalf("Len %d, Slots %d; want 3, 3", h.Len(), h.Slots())
	}
	if size := fileSize(t, path); size != HeaderSize+3*record.RecordSize {
		t.Fatalf("file size %d", size)
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	h, _ := newHeap(t)
	off, _ := h.Alloc()
	placeholder, _ := h.Read(off)
	if !placeholder.Live || placeholder.Links != record.NoLinks() {
		t.Fatalf("fresh slot = %+v, want live placeholder without links", placeholder)
	}

	want := record.Record{Live: true, Visit: visit(42), Links: record.Links{
		PrevSpecies: 64, NextSpecies: record.NilOffset, PrevDoctor: 262, NextDoctor: 460,
	}}
	if err := h.Write(off, want); err != nil {
		t.Fatal(err)
	}
	got, err := h.Read(off)
	if err != nil {
		t.Fatal(err)
	}
	if got.Visit.ID != 42 || got.Links != want.Links || !got.Visit.At.Equal(want.Visit.At) {
		t.Fatalf("Read = %+v, want %+v", got, want)
	}

	want.Visit.PetName = "Мурзик" // update in place
	h.Write(off, want)
	if got, _ := h.Read(off); got.Visit.PetName != "Мурзик" {
		t.Fatalf("update in place lost: %q", got.Visit.PetName)
	}
}

func TestFreeListIsLIFOAndReused(t *testing.T) {
	h, _ := newHeap(t)
	a := insert(t, h, 1)
	b := insert(t, h, 2)
	c := insert(t, h, 3)

	for _, off := range []int64{a, b, c} {
		if err := h.Free(off); err != nil {
			t.Fatal(err)
		}
	}
	if h.Len() != 0 || h.Slots() != 3 {
		t.Fatalf("after freeing all: Len %d, Slots %d; want 0, 3", h.Len(), h.Slots())
	}

	// The last freed slot is reused first; only then the file grows.
	for _, want := range []int64{c, b, a, HeaderSize + 3*record.RecordSize} {
		if off := insert(t, h, 9); off != want {
			t.Fatalf("Alloc = %d, want %d", off, want)
		}
	}
	if h.Slots() != 4 {
		t.Fatalf("Slots = %d, want 4", h.Slots())
	}
}

func TestFreeWipesData(t *testing.T) {
	h, _ := newHeap(t)
	off := insert(t, h, 7)
	h.Free(off)
	r, err := h.Read(off)
	if err != nil {
		t.Fatal(err)
	}
	if r.Live || r.Visit.ID != 0 || r.Visit.PetName != "" {
		t.Fatalf("freed slot still holds data: %+v", r)
	}
}

func TestFreeAndWriteRejectFreeSlots(t *testing.T) {
	h, _ := newHeap(t)
	off := insert(t, h, 1)
	h.Free(off)

	if err := h.Free(off); !errors.Is(err, ErrNotLive) {
		t.Errorf("double Free: want ErrNotLive, got %v", err)
	}
	live := record.Record{Live: true, Visit: visit(1)}
	if err := h.Write(off, live); !errors.Is(err, ErrNotLive) {
		t.Errorf("Write to free slot: want ErrNotLive, got %v", err)
	}
	other := insert(t, h, 2) // takes off back from the free list
	if err := h.Write(other, record.Record{Live: false}); !errors.Is(err, ErrNotLive) {
		t.Errorf("Write with Live=false: want ErrNotLive, got %v", err)
	}
}

func TestBadOffsets(t *testing.T) {
	h, _ := newHeap(t)
	insert(t, h, 1)
	insert(t, h, 2)
	for _, off := range []int64{-1, 0, 10, HeaderSize + 1, HeaderSize + 2*record.RecordSize} {
		if _, err := h.Read(off); !errors.Is(err, ErrBadOffset) {
			t.Errorf("Read(%d): want ErrBadOffset, got %v", off, err)
		}
		if err := h.Free(off); !errors.Is(err, ErrBadOffset) {
			t.Errorf("Free(%d): want ErrBadOffset, got %v", off, err)
		}
	}
}

func TestAllocDetectsBrokenFreeList(t *testing.T) {
	h, _ := newHeap(t)
	off := insert(t, h, 1)
	h.freeHead = off // simulate damage: free list points at a live slot
	if _, err := h.Alloc(); !errors.Is(err, ErrBadFile) {
		t.Fatalf("want ErrBadFile, got %v", err)
	}
}

func TestScan(t *testing.T) {
	h, _ := newHeap(t)
	const n = 2500 // more than two scan chunks
	offs := make([]int64, n)
	for i := range offs {
		offs[i] = insert(t, h, int32(i+1))
	}
	for i := 0; i < n; i += 3 {
		h.Free(offs[i])
	}

	var ids []int32
	var prev int64 = -1
	err := h.Scan(func(off int64, r record.Record) bool {
		if off <= prev {
			t.Fatalf("Scan out of file order: %d after %d", off, prev)
		}
		prev = off
		ids = append(ids, r.Visit.ID)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(ids)) != h.Len() {
		t.Fatalf("Scan saw %d records, Len = %d", len(ids), h.Len())
	}
	for _, id := range ids {
		if (id-1)%3 == 0 {
			t.Fatalf("Scan returned freed record %d", id)
		}
	}

	seen := 0
	h.Scan(func(int64, record.Record) bool { seen++; return seen < 5 })
	if seen != 5 {
		t.Fatalf("Scan did not stop early: saw %d", seen)
	}
}

// TestRandomAgainstMap runs random inserts, updates and frees against the
// heap and a map of offset -> id, reopening the file from time to time.
func TestRandomAgainstMap(t *testing.T) {
	h, path := newHeap(t)
	want := map[int64]int32{}
	var live []int64
	peak := 0 // most records alive at the same time
	rng := rand.New(rand.NewPCG(3, 4))

	for op := 0; op < 10_000; op++ {
		switch {
		case len(live) == 0 || rng.IntN(3) == 0:
			id := int32(op + 1)
			off := insert(t, h, id)
			if _, dup := want[off]; dup {
				t.Fatalf("op %d: Alloc returned live offset %d", op, off)
			}
			want[off] = id
			live = append(live, off)
			peak = max(peak, len(live))
		case rng.IntN(2) == 0:
			i := rng.IntN(len(live))
			off := live[i]
			if err := h.Free(off); err != nil {
				t.Fatalf("op %d Free: %v", op, err)
			}
			delete(want, off)
			live[i] = live[len(live)-1]
			live = live[:len(live)-1]
		default:
			off := live[rng.IntN(len(live))]
			id := int32(-op - 1)
			if err := h.Write(off, record.Record{Live: true, Visit: visit(id)}); err != nil {
				t.Fatalf("op %d Write: %v", op, err)
			}
			want[off] = id
		}
		if op%2500 == 2499 {
			h = reopen(t, h, path)
		}
	}

	got := map[int64]int32{}
	if err := h.Scan(func(off int64, r record.Record) bool { got[off] = r.Visit.ID; return true }); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) || h.Len() != uint64(len(want)) {
		t.Fatalf("Scan %d, Len %d, map %d", len(got), h.Len(), len(want))
	}
	for off, id := range want {
		if got[off] != id {
			t.Fatalf("offset %d: heap id %d, map id %d", off, got[off], id)
		}
	}
	// The file grows only when the free list is empty, so it never has more
	// slots than the peak number of live records.
	if h.Slots() != uint64(peak) {
		t.Fatalf("Slots = %d, want peak live count %d: free slots are not reused", h.Slots(), peak)
	}
}
