package db

import (
	"errors"
	"testing"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

func TestInsertGet(t *testing.T) {
	d, _ := newDB(t)
	v := visit(7, "  Кошка ", "Иванова А. П.")
	mustInsert(t, d, v)

	got, err := d.Get(7)
	if err != nil {
		t.Fatal(err)
	}
	if got.Species != "Кошка" {
		t.Errorf("spaces not trimmed: %q", got.Species)
	}
	if got.PetName != v.PetName || got.Price != v.Price || !got.At.Equal(v.At) {
		t.Errorf("Get = %+v, want %+v", got, v)
	}
	if _, err := d.Get(8); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing: want ErrNotFound, got %v", err)
	}
}

func TestInsertRejectsDuplicateAndInvalid(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"))

	if err := d.Insert(visit(1, "Собака", "Петров")); !errors.Is(err, ErrDuplicateKey) {
		t.Errorf("duplicate: want ErrDuplicateKey, got %v", err)
	}
	bad := visit(2, "Кошка", "Иванова")
	bad.Price = -5
	if err := d.Insert(bad); !errors.Is(err, record.ErrInvalid) {
		t.Errorf("invalid: want record.ErrInvalid, got %v", err)
	}
	if d.Len() != 1 {
		t.Errorf("rejected inserts changed the table: Len = %d", d.Len())
	}
	checkIntegrity(t, d)
}

func TestDeleteByIDFromEveryChainPosition(t *testing.T) {
	// 1..4 are cats, so the cat chain is 4 -> 3 -> 2 -> 1 (newest first).
	tests := []struct {
		name string
		del  int32
	}{
		{"head", 4}, {"middle", 3}, {"tail", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := newDB(t)
			for id := int32(1); id <= 4; id++ {
				mustInsert(t, d, visit(id, "Кошка", "Иванова"))
			}
			mustInsert(t, d, visit(5, "Собака", "Петров"))

			if err := d.DeleteByID(tc.del); err != nil {
				t.Fatal(err)
			}
			if _, err := d.Get(tc.del); !errors.Is(err, ErrNotFound) {
				t.Fatalf("deleted visit still found: %v", err)
			}
			checkIntegrity(t, d)
			checkIntegrity(t, reopen(t, d))
		})
	}
}

func TestDeleteLastOfValueRemovesItFromIndex(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Попугай", "Сидорова"), visit(2, "Кошка", "Иванова"))
	d.DeleteByID(1)
	if d.species.Len() != 1 || d.doctor.Len() != 1 {
		t.Fatalf("index sizes %d, %d; want 1, 1", d.species.Len(), d.doctor.Len())
	}
	if err := d.DeleteByID(1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: want ErrNotFound, got %v", err)
	}
}

func TestUpdateInPlace(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"), visit(2, "Кошка", "Иванова"))

	v := visit(1, "Кошка", "Иванова")
	v.PetName, v.Price, v.Status = "Мурзик", 2500, record.StatusVisited
	if err := d.Update(1, v); err != nil {
		t.Fatal(err)
	}
	got, _ := d.Get(1)
	if got.PetName != "Мурзик" || got.Price != 2500 || got.Status != record.StatusVisited {
		t.Fatalf("Update lost fields: %+v", got)
	}
	checkIntegrity(t, d)
}

func TestUpdateMovesBetweenChains(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d,
		visit(1, "Кошка", "Иванова"),
		visit(2, "Кошка", "Иванова"),
		visit(3, "Кошка", "Иванова"),
		visit(4, "Собака", "Петров"),
	)
	// 2 is in the middle of both chains; move it to the dog/Petrov chains.
	if err := d.Update(2, visit(2, "Собака", "Петров")); err != nil {
		t.Fatal(err)
	}
	cats, _ := d.FindBy(FieldSpecies, "Кошка")
	dogs, _ := d.FindBy(FieldSpecies, "Собака")
	if !equalIDs(ids(cats), []int32{1, 3}) || !equalIDs(ids(dogs), []int32{2, 4}) {
		t.Fatalf("cats %v, dogs %v", ids(cats), ids(dogs))
	}
	checkIntegrity(t, d)

	// Only the doctor changes: a new value creates a new chain.
	if err := d.Update(1, visit(1, "Кошка", "Сидорова")); err != nil {
		t.Fatal(err)
	}
	checkIntegrity(t, reopen(t, d))
}

func TestUpdateChangesID(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"), visit(2, "Собака", "Петров"))

	if err := d.Update(1, visit(10, "Кошка", "Иванова")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Get(1); !errors.Is(err, ErrNotFound) {
		t.Errorf("old ID still found: %v", err)
	}
	if _, err := d.Get(10); err != nil {
		t.Errorf("new ID not found: %v", err)
	}
	if err := d.Update(10, visit(2, "Кошка", "Иванова")); !errors.Is(err, ErrDuplicateKey) {
		t.Errorf("update to taken ID: want ErrDuplicateKey, got %v", err)
	}
	checkIntegrity(t, d)
}

func TestUpdateErrors(t *testing.T) {
	d, _ := newDB(t)
	mustInsert(t, d, visit(1, "Кошка", "Иванова"))
	if err := d.Update(5, visit(5, "Кошка", "Иванова")); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: want ErrNotFound, got %v", err)
	}
	bad := visit(1, "", "Иванова")
	if err := d.Update(1, bad); !errors.Is(err, record.ErrInvalid) {
		t.Errorf("invalid: want record.ErrInvalid, got %v", err)
	}
}
