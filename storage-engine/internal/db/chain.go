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

// link puts the record at off at the head of its chain. O(1).
//
// It updates the old head on disk and the index; the links of rec are set
// in memory and the caller writes rec itself.
func (d *DB) link(c chain, off int64, rec *record.Record) error {
	key := []byte(c.value(rec.Visit))
	head, ok, err := c.idx.Get(key)
	if err != nil {
		return err
	}
	*c.prev(&rec.Links) = record.NilOffset
	*c.next(&rec.Links) = record.NilOffset
	if ok {
		*c.next(&rec.Links) = head
		h, err := d.heap.Read(head)
		if err != nil {
			return err
		}
		*c.prev(&h.Links) = off
		if err := d.heap.Write(head, h); err != nil {
			return err
		}
	}
	return c.idx.Put(key, off)
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
