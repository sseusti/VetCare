package db

import (
	"github.com/sseusti/VetCare/storage-engine/internal/hashfile"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// chain describes one secondary index: the hash file that maps a value to
// the head of its chain, which field of a visit is indexed and which pair of
// links inside a record holds the chain.
type chain struct {
	idx   *hashfile.File
	value func(v record.Visit) string
	prev  func(l *record.Links) *int64
	next  func(l *record.Links) *int64
}

func (d *DB) speciesChain() chain {
	return chain{
		idx:   d.species,
		value: func(v record.Visit) string { return v.Species },
		prev:  func(l *record.Links) *int64 { return &l.PrevSpecies },
		next:  func(l *record.Links) *int64 { return &l.NextSpecies },
	}
}

func (d *DB) doctorChain() chain {
	return chain{
		idx:   d.doctor,
		value: func(v record.Visit) string { return v.Doctor },
		prev:  func(l *record.Links) *int64 { return &l.PrevDoctor },
		next:  func(l *record.Links) *int64 { return &l.NextDoctor },
	}
}

func (d *DB) chains() []chain { return []chain{d.speciesChain(), d.doctorChain()} }

// prepare sets the links of rec for the head of its chain, in memory only,
// and returns the current head (NilOffset if the chain is empty). O(1).
//
// Nothing is written: the caller writes rec first and then calls attach, so
// an index never points at a record that is not on disk yet.
func (d *DB) prepare(c chain, rec *record.Record) (int64, error) {
	head, ok, err := c.idx.Get([]byte(c.value(rec.Visit)))
	if err != nil {
		return 0, err
	}
	*c.prev(&rec.Links) = record.NilOffset
	*c.next(&rec.Links) = record.NilOffset
	if !ok {
		return record.NilOffset, nil
	}
	*c.next(&rec.Links) = head
	return head, nil
}

// attach makes the record at off, already written with the links from
// prepare, the head of its chain: the old head points back at it and the
// index points at it. O(1): at most one record write and one index write.
func (d *DB) attach(c chain, off int64, rec record.Record, head int64) error {
	if head != record.NilOffset {
		h, err := d.heap.Read(head)
		if err != nil {
			return err
		}
		*c.prev(&h.Links) = off
		if err := d.heap.Write(head, h); err != nil {
			return err
		}
	}
	return c.idx.Put([]byte(c.value(rec.Visit)), off)
}

// unlink removes the record at off from its chain by connecting its
// neighbours to each other. O(1): at most two record writes and one index
// write. The links of rec are reset in memory.
func (d *DB) unlink(c chain, rec *record.Record) error {
	prev, next := *c.prev(&rec.Links), *c.next(&rec.Links)

	if prev != record.NilOffset {
		p, err := d.heap.Read(prev)
		if err != nil {
			return err
		}
		*c.next(&p.Links) = next
		if err := d.heap.Write(prev, p); err != nil {
			return err
		}
	} else {
		// rec was the head: the index must point at the next record, or
		// forget the value if the chain is now empty.
		key := []byte(c.value(rec.Visit))
		if next != record.NilOffset {
			if err := c.idx.Put(key, next); err != nil {
				return err
			}
		} else if _, err := c.idx.Delete(key); err != nil {
			return err
		}
	}

	if next != record.NilOffset {
		n, err := d.heap.Read(next)
		if err != nil {
			return err
		}
		*c.prev(&n.Links) = prev
		if err := d.heap.Write(next, n); err != nil {
			return err
		}
	}

	*c.prev(&rec.Links) = record.NilOffset
	*c.next(&rec.Links) = record.NilOffset
	return nil
}

// walk calls fn for every record in the chain of value, head first.
// O(1 + k), where k is the length of the chain.
func (d *DB) walk(c chain, value string, fn func(off int64, r record.Record) error) error {
	off, ok, err := c.idx.Get([]byte(value))
	if err != nil || !ok {
		return err
	}
	for off != record.NilOffset {
		r, err := d.heap.Read(off)
		if err != nil {
			return err
		}
		next := *c.next(&r.Links)
		if err := fn(off, r); err != nil {
			return err
		}
		off = next
	}
	return nil
}
