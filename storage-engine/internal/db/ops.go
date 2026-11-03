package db

import (
	"fmt"
	"strings"

	"github.com/sseusti/VetCare/storage-engine/internal/hashfile"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// Insert adds a visit. O(1) on average: a key lookup for uniqueness, one
// slot allocation, one record write and a head insert into two chains.
//
// The record is written before any index points at it: first the record
// with its links, then the two chains, then the key index last.
func (d *DB) Insert(v record.Visit) error {
	if d.heap == nil {
		return ErrClosed
	}
	v = normalize(v)
	if err := record.Validate(v); err != nil {
		return err
	}
	if _, ok, err := d.key.Get(hashfile.Int32Key(v.ID)); err != nil {
		return err
	} else if ok {
		return fmt.Errorf("%w: %d", ErrDuplicateKey, v.ID)
	}

	off, err := d.heap.Alloc()
	if err != nil {
		return err
	}
	rec := record.Record{Live: true, Visit: v, Links: record.NoLinks()}
	chains := d.chains()
	heads := make([]int64, len(chains))
	for i, c := range chains {
		if heads[i], err = d.prepare(c, &rec); err != nil {
			return err
		}
	}
	if err := d.heap.Write(off, rec); err != nil {
		return err
	}
	for i, c := range chains {
		if err := d.attach(c, off, rec, heads[i]); err != nil {
			return err
		}
	}
	return d.key.Put(hashfile.Int32Key(v.ID), off)
}

// Get returns the visit with the given ID. O(1) on average.
func (d *DB) Get(id int32) (record.Visit, error) {
	_, rec, err := d.find(id)
	return rec.Visit, err
}

// DeleteByID deletes the visit with the given ID. O(1) on average.
func (d *DB) DeleteByID(id int32) error {
	off, rec, err := d.find(id)
	if err != nil {
		return err
	}
	for _, c := range d.chains() {
		if err := d.unlink(c, &rec); err != nil {
			return err
		}
	}
	if _, err := d.key.Delete(hashfile.Int32Key(id)); err != nil {
		return err
	}
	return d.heap.Free(off)
}

// Update replaces the visit with the given ID by v, in place. O(1) on
// average. If v.ID differs, the key index is moved; if species or doctor
// differ, the record is moved to the other chain.
func (d *DB) Update(id int32, v record.Visit) error {
	v = normalize(v)
	if err := record.Validate(v); err != nil {
		return err
	}
	off, rec, err := d.find(id)
	if err != nil {
		return err
	}
	if v.ID != id {
		if _, ok, err := d.key.Get(hashfile.Int32Key(v.ID)); err != nil {
			return err
		} else if ok {
			return fmt.Errorf("%w: %d", ErrDuplicateKey, v.ID)
		}
	}

	// Chains whose value changes: leave the old chain, write the record with
	// the new links, then join the new chain — as in Insert, the record is on
	// disk before an index points at it.
	var moved []chain
	for _, c := range d.chains() {
		if c.value(rec.Visit) != c.value(v) {
			moved = append(moved, c)
		}
	}
	for _, c := range moved {
		if err := d.unlink(c, &rec); err != nil {
			return err
		}
	}
	rec.Visit = v
	heads := make([]int64, len(moved))
	for i, c := range moved {
		if heads[i], err = d.prepare(c, &rec); err != nil {
			return err
		}
	}
	if err := d.heap.Write(off, rec); err != nil {
		return err
	}
	for i, c := range moved {
		if err := d.attach(c, off, rec, heads[i]); err != nil {
			return err
		}
	}

	if v.ID != id {
		if _, err := d.key.Delete(hashfile.Int32Key(id)); err != nil {
			return err
		}
		return d.key.Put(hashfile.Int32Key(v.ID), off)
	}
	return nil
}

// find returns the offset and record of the visit with the given ID.
func (d *DB) find(id int32) (int64, record.Record, error) {
	if d.heap == nil {
		return 0, record.Record{}, ErrClosed
	}
	off, ok, err := d.key.Get(hashfile.Int32Key(id))
	if err != nil {
		return 0, record.Record{}, err
	}
	if !ok {
		return 0, record.Record{}, fmt.Errorf("%w: %d", ErrNotFound, id)
	}
	rec, err := d.heap.Read(off)
	if err != nil {
		return 0, record.Record{}, err
	}
	if !rec.Live || rec.Visit.ID != id {
		return 0, record.Record{}, fmt.Errorf("%w: key %d points at a wrong record", ErrCorrupt, id)
	}
	return off, rec, nil
}

// normalize trims spaces around text fields, so " Кошка" and "Кошка" are
// the same value for the indexes.
func normalize(v record.Visit) record.Visit {
	v.PetName = strings.TrimSpace(v.PetName)
	v.Species = strings.TrimSpace(v.Species)
	v.Doctor = strings.TrimSpace(v.Doctor)
	return v
}
