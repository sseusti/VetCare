package db

import (
	"testing"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// prepare must not touch the disk: the record is written first, and only
// then attach changes the old head and the index. Otherwise a crash between
// the two steps would leave an index pointing at a record that is not there.
func TestPrepareWritesNothing(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванов"))
	headOff, ok, err := d.species.Get([]byte("Кошка"))
	if err != nil || !ok {
		t.Fatalf("species head: %v %v", ok, err)
	}
	before, err := d.heap.Read(headOff)
	if err != nil {
		t.Fatal(err)
	}

	rec := record.Record{Live: true, Visit: visit(2, "Кошка", "Иванов"), Links: record.NoLinks()}
	head, err := d.prepare(d.speciesChain(), &rec)
	if err != nil {
		t.Fatal(err)
	}
	if head != headOff || rec.Links.NextSpecies != headOff || rec.Links.PrevSpecies != record.NilOffset {
		t.Fatalf("links of the new record: head %d, links %+v", head, rec.Links)
	}

	after, err := d.heap.Read(headOff)
	if err != nil {
		t.Fatal(err)
	}
	if after.Links != before.Links {
		t.Fatalf("prepare changed the old head on disk: %+v -> %+v", before.Links, after.Links)
	}
	if off, _, _ := d.species.Get([]byte("Кошка")); off != headOff {
		t.Fatalf("prepare moved the index to %d", off)
	}
}

// After Insert the new record heads both chains and the old heads point back.
func TestInsertLinksNewHeadBothWays(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванов"), visit(2, "Кошка", "Иванов"))

	newHead, _, _ := d.species.Get([]byte("Кошка"))
	oldHead, _, _ := d.key.Get(keyOf(1))
	rec, err := d.heap.Read(newHead)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Visit.ID != 2 || rec.Links.NextSpecies != oldHead || rec.Links.NextDoctor != oldHead {
		t.Fatalf("new head %+v does not lead to the old one at %d", rec, oldHead)
	}
	old, err := d.heap.Read(oldHead)
	if err != nil {
		t.Fatal(err)
	}
	if old.Links.PrevSpecies != newHead || old.Links.PrevDoctor != newHead {
		t.Fatalf("old head links %+v do not point back at %d", old.Links, newHead)
	}
	checkIntegrity(t, d)
}
