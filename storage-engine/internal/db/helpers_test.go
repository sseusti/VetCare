package db

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/sseusti/VetCare/storage-engine/internal/hashfile"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

func newDB(t *testing.T) (*DB, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vet")
	d, err := Create(dir)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d, dir
}

func reopen(t *testing.T, d *DB) *DB {
	t.Helper()
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	d2, err := Open(d.dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d2.Close() })
	return d2
}

func visit(id int32, species, doctor string) record.Visit {
	return record.Visit{
		ID:      id,
		PetName: "Барсик",
		Species: species,
		Doctor:  doctor,
		At:      time.Unix(1_790_000_000+int64(id)*3600, 0),
		Status:  record.StatusScheduled,
		Price:   1500,
	}
}

func mustInsert(t *testing.T, d *DB, vs ...record.Visit) {
	t.Helper()
	for _, v := range vs {
		if err := d.Insert(v); err != nil {
			t.Fatalf("Insert(%d): %v", v.ID, err)
		}
	}
}

// ids returns sorted IDs of visits, for order-independent comparisons.
func ids(vs []record.Visit) []int32 {
	out := make([]int32, len(vs))
	for i, v := range vs {
		out[i] = v.ID
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func equalIDs(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// checkIntegrity verifies every invariant that ties the files together:
// the key index points at the right live records, and every species and
// doctor chain is a correct doubly linked list holding exactly the records
// with that value.
func checkIntegrity(t *testing.T, d *DB) {
	t.Helper()
	bySpecies := map[string][]int32{}
	byDoctor := map[string][]int32{}
	n := 0
	err := d.heap.Scan(func(off int64, r record.Record) bool {
		n++
		got, ok, err := d.key.Get(keyOf(r.Visit.ID))
		if err != nil || !ok || got != off {
			t.Fatalf("key index for %d: %d, %v, %v; record at %d", r.Visit.ID, got, ok, err, off)
		}
		bySpecies[r.Visit.Species] = append(bySpecies[r.Visit.Species], r.Visit.ID)
		byDoctor[r.Visit.Doctor] = append(byDoctor[r.Visit.Doctor], r.Visit.ID)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if uint64(n) != d.key.Len() {
		t.Fatalf("%d records but %d keys", n, d.key.Len())
	}
	checkChains(t, d, d.speciesChain(), bySpecies)
	checkChains(t, d, d.doctorChain(), byDoctor)
}

func checkChains(t *testing.T, d *DB, c chain, want map[string][]int32) {
	t.Helper()
	if c.idx.Len() != uint64(len(want)) {
		t.Fatalf("index has %d values, records have %d", c.idx.Len(), len(want))
	}
	for value, wantIDs := range want {
		var got []int32
		prev := record.NilOffset
		err := d.walk(c, value, func(off int64, r record.Record) error {
			if *c.prev(&r.Links) != prev {
				t.Fatalf("chain %q: record %d has prev %d, want %d", value, r.Visit.ID, *c.prev(&r.Links), prev)
			}
			if c.value(r.Visit) != value {
				t.Fatalf("chain %q holds record %d with value %q", value, r.Visit.ID, c.value(r.Visit))
			}
			got = append(got, r.Visit.ID)
			prev = off
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
		sort.Slice(wantIDs, func(i, j int) bool { return wantIDs[i] < wantIDs[j] })
		if !equalIDs(got, wantIDs) {
			t.Fatalf("chain %q = %v, want %v", value, got, wantIDs)
		}
	}
}

func keyOf(id int32) []byte { return hashfile.Int32Key(id) }
