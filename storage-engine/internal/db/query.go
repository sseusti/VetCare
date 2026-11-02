package db

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// Field is a visit field that can be searched or deleted by.
type Field int

// Searchable fields. ID, species and doctor have indexes; the rest are
// searched by a full scan.
const (
	FieldID Field = iota
	FieldSpecies
	FieldDoctor
	FieldPetName
	FieldStatus
)

var fieldNames = map[Field]string{
	FieldID:      "ID",
	FieldSpecies: "Вид",
	FieldDoctor:  "Врач",
	FieldPetName: "Кличка",
	FieldStatus:  "Статус",
}

func (f Field) String() string {
	if n, ok := fieldNames[f]; ok {
		return n
	}
	return fmt.Sprintf("Field(%d)", int(f))
}

// Indexed reports whether searching by f avoids a full scan.
func (f Field) Indexed() bool { return f == FieldID || f == FieldSpecies || f == FieldDoctor }

// Fields returns every searchable field in display order.
func Fields() []Field {
	return []Field{FieldID, FieldSpecies, FieldDoctor, FieldPetName, FieldStatus}
}

// FindBy returns every visit whose field equals value.
//
//	ID:              O(1), key index
//	species, doctor: O(1 + k), walk of one chain
//	pet name, status: O(n), full scan (no index)
func (d *DB) FindBy(f Field, value string) ([]record.Visit, error) {
	if d.heap == nil {
		return nil, ErrClosed
	}
	value = strings.TrimSpace(value)
	var out []record.Visit
	collect := func(_ int64, r record.Record) error {
		out = append(out, r.Visit)
		return nil
	}

	switch f {
	case FieldID:
		id, err := parseID(value)
		if err != nil {
			return nil, err
		}
		v, err := d.Get(id)
		if err != nil {
			if isNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		return []record.Visit{v}, nil
	case FieldSpecies:
		return out, d.walk(d.speciesChain(), value, collect)
	case FieldDoctor:
		return out, d.walk(d.doctorChain(), value, collect)
	case FieldPetName:
		return out, d.Scan(func(v record.Visit) bool {
			if v.PetName == value {
				out = append(out, v)
			}
			return true
		})
	case FieldStatus:
		s, err := record.ParseStatus(value)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadValue, err)
		}
		return out, d.Scan(func(v record.Visit) bool {
			if v.Status == s {
				out = append(out, v)
			}
			return true
		})
	}
	return nil, fmt.Errorf("%w: unknown field %v", ErrBadValue, f)
}

// DeleteBy deletes every visit whose field equals value and returns how many
// were deleted. Each deletion is O(1), so the total is O(k) for indexed
// fields plus the cost of FindBy.
func (d *DB) DeleteBy(f Field, value string) (int, error) {
	found, err := d.FindBy(f, value)
	if err != nil {
		return 0, err
	}
	for i, v := range found {
		if err := d.DeleteByID(v.ID); err != nil {
			return i, err
		}
	}
	return len(found), nil
}

func parseID(s string) (int32, error) {
	id, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: ID must be an integer, got %q", ErrBadValue, s)
	}
	return int32(id), nil
}

func isNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
