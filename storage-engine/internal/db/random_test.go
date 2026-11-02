package db

import (
	"errors"
	"math/rand/v2"
	"sort"
	"testing"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// TestRandomAgainstModel runs thousands of random inserts, updates and
// deletes (by ID and by field) on the database and on a plain map at the
// same time. After every batch it compares search results and checks every
// invariant of the files, and from time to time reopens the database.
func TestRandomAgainstModel(t *testing.T) {
	d, _ := newDB(t)
	model := map[int32]record.Visit{}
	rng := rand.New(rand.NewPCG(5, 6))
	species := []string{"Кошка", "Собака", "Попугай", "Хомяк", "Кролик"}
	doctors := []string{"Иванова", "Петров", "Сидорова", "Кузнецов"}
	randVisit := func(id int32) record.Visit {
		return visit(id, species[rng.IntN(len(species))], doctors[rng.IntN(len(doctors))])
	}

	const ops = 6000
	for op := 0; op < ops; op++ {
		id := int32(rng.IntN(400) + 1)
		_, exists := model[id]
		switch r := rng.IntN(10); {
		case r < 5: // insert
			v := randVisit(id)
			err := d.Insert(v)
			if exists != errors.Is(err, ErrDuplicateKey) || (!exists && err != nil) {
				t.Fatalf("op %d Insert(%d): exists %v, err %v", op, id, exists, err)
			}
			if !exists {
				model[id] = v
			}
		case r < 7: // delete by ID
			err := d.DeleteByID(id)
			if exists != (err == nil) {
				t.Fatalf("op %d DeleteByID(%d): exists %v, err %v", op, id, exists, err)
			}
			delete(model, id)
		case r < 9: // update, sometimes with a new ID
			if !exists {
				continue
			}
			v := randVisit(id)
			if rng.IntN(4) == 0 {
				v.ID = int32(rng.IntN(400) + 1)
			}
			_, taken := model[v.ID]
			err := d.Update(id, v)
			if v.ID != id && taken {
				if !errors.Is(err, ErrDuplicateKey) {
					t.Fatalf("op %d Update to taken ID: %v", op, err)
				}
				continue
			}
			if err != nil {
				t.Fatalf("op %d Update(%d): %v", op, id, err)
			}
			delete(model, id)
			model[v.ID] = v
		default: // delete by species or doctor, rarely
			if rng.IntN(5) != 0 {
				continue
			}
			f, value := FieldSpecies, species[rng.IntN(len(species))]
			if rng.IntN(2) == 0 {
				f, value = FieldDoctor, doctors[rng.IntN(len(doctors))]
			}
			want := 0
			for k, v := range model {
				if (f == FieldSpecies && v.Species == value) || (f == FieldDoctor && v.Doctor == value) {
					delete(model, k)
					want++
				}
			}
			n, err := d.DeleteBy(f, value)
			if err != nil || n != want {
				t.Fatalf("op %d DeleteBy(%v, %s) = %d, %v; want %d", op, f, value, n, err, want)
			}
		}

		if op%500 == 499 {
			compareWithModel(t, d, model, species, doctors)
			checkIntegrity(t, d)
		}
		if op%2000 == 1999 {
			d = reopen(t, d)
		}
	}
	compareWithModel(t, d, model, species, doctors)
	checkIntegrity(t, d)
}

func compareWithModel(t *testing.T, d *DB, model map[int32]record.Visit, species, doctors []string) {
	t.Helper()
	if d.Len() != uint64(len(model)) {
		t.Fatalf("Len = %d, model %d", d.Len(), len(model))
	}
	for id, v := range model {
		got, err := d.Get(id)
		if err != nil || got.Species != v.Species || got.Doctor != v.Doctor {
			t.Fatalf("Get(%d) = %+v, %v; model %+v", id, got, err, v)
		}
	}
	for _, s := range species {
		compareSearch(t, d, model, FieldSpecies, s, func(v record.Visit) bool { return v.Species == s })
	}
	for _, doc := range doctors {
		compareSearch(t, d, model, FieldDoctor, doc, func(v record.Visit) bool { return v.Doctor == doc })
	}
}

func compareSearch(t *testing.T, d *DB, model map[int32]record.Visit, f Field, value string, match func(record.Visit) bool) {
	t.Helper()
	var want []int32
	for id, v := range model {
		if match(v) {
			want = append(want, id)
		}
	}
	got, err := d.FindBy(f, value)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if !equalIDs(ids(got), want) {
		t.Fatalf("FindBy(%v, %s) = %v, model %v", f, value, ids(got), want)
	}
}
