package db

import (
	"errors"
	"testing"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

func seed(t *testing.T) *DB {
	t.Helper()
	d, _ := newDB(t)
	mustInsert(t, d,
		visit(1, "Кошка", "Иванова"),
		visit(2, "Собака", "Иванова"),
		visit(3, "Кошка", "Петров"),
		visit(4, "Попугай", "Петров"),
		visit(5, "Кошка", "Сидорова"),
	)
	v := visit(6, "Собака", "Сидорова")
	v.PetName, v.Status = "Шарик", record.StatusNoShow
	mustInsert(t, d, v)
	return d
}

func TestFindBy(t *testing.T) {
	d := seed(t)
	tests := []struct {
		field Field
		value string
		want  []int32
	}{
		{FieldID, "3", []int32{3}},
		{FieldID, " 42 ", nil},
		{FieldSpecies, "Кошка", []int32{1, 3, 5}},
		{FieldSpecies, " Собака ", []int32{2, 6}},
		{FieldSpecies, "Хомяк", nil},
		{FieldDoctor, "Петров", []int32{3, 4}},
		{FieldPetName, "Шарик", []int32{6}},
		{FieldPetName, "Барсик", []int32{1, 2, 3, 4, 5}},
		{FieldStatus, "Не пришёл", []int32{6}},
	}
	for _, tc := range tests {
		t.Run(tc.field.String()+"="+tc.value, func(t *testing.T) {
			got, err := d.FindBy(tc.field, tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if !equalIDs(ids(got), tc.want) {
				t.Fatalf("got %v, want %v", ids(got), tc.want)
			}
		})
	}
}

func TestFindByBadValues(t *testing.T) {
	d := seed(t)
	for _, tc := range []struct {
		field Field
		value string
	}{
		{FieldID, "abc"},
		{FieldID, "99999999999"},
		{FieldStatus, "Улетел"},
		{Field(99), "x"},
	} {
		if _, err := d.FindBy(tc.field, tc.value); !errors.Is(err, ErrBadValue) {
			t.Errorf("FindBy(%v, %q): want ErrBadValue, got %v", tc.field, tc.value, err)
		}
	}
}

func TestDeleteBy(t *testing.T) {
	d := seed(t)

	n, err := d.DeleteBy(FieldSpecies, "Кошка")
	if err != nil || n != 3 {
		t.Fatalf("DeleteBy species = %d, %v; want 3", n, err)
	}
	if left, _ := d.FindBy(FieldSpecies, "Кошка"); len(left) != 0 {
		t.Fatalf("cats left: %v", ids(left))
	}
	checkIntegrity(t, d)

	if n, _ := d.DeleteBy(FieldDoctor, "Сидорова"); n != 1 {
		t.Fatalf("DeleteBy doctor = %d, want 1 (only visit 6 is left)", n)
	}
	if n, _ := d.DeleteBy(FieldID, "2"); n != 1 {
		t.Fatalf("DeleteBy ID = %d, want 1", n)
	}
	if n, _ := d.DeleteBy(FieldPetName, "Нет такого"); n != 0 {
		t.Fatalf("DeleteBy nothing = %d", n)
	}
	if d.Len() != 1 {
		t.Fatalf("Len = %d, want 1", d.Len())
	}
	if _, err := d.DeleteBy(FieldID, "x"); !errors.Is(err, ErrBadValue) {
		t.Fatalf("DeleteBy bad value: %v", err)
	}
	checkIntegrity(t, reopen(t, d))
}

func TestFieldNames(t *testing.T) {
	for _, f := range Fields() {
		if f.String() == "" {
			t.Errorf("field %d has no name", f)
		}
	}
	if Field(99).String() != "Field(99)" {
		t.Errorf("unknown field name = %q", Field(99).String())
	}
	if !FieldDoctor.Indexed() || FieldPetName.Indexed() {
		t.Error("Indexed is wrong")
	}
}
