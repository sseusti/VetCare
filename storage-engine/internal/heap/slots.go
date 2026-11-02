package heap

import (
	"fmt"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// scanChunk is how many slots Scan reads at once.
const scanChunk = 1024

// Alloc reserves a slot for a new record and returns its offset. O(1).
//
// It takes the head of the free list if there is one, otherwise it appends
// a slot at the end of the file. The slot is marked live with an empty
// visit; the caller fills it with Write.
func (h *File) Alloc() (int64, error) {
	if h.f == nil {
		return 0, ErrClosed
	}

	var off int64
	if h.freeHead != record.NilOffset {
		off = h.freeHead
		free, err := h.Read(off)
		if err != nil {
			return 0, err
		}
		if free.Live {
			return 0, fmt.Errorf("%w: free list points at a live slot %d", ErrBadFile, off)
		}
		h.freeHead = free.Links.NextSpecies
	} else {
		off = slotOffset(h.slots)
		h.slots++
	}

	placeholder := record.Record{Live: true, Links: record.NoLinks()}
	if _, err := h.f.WriteAt(record.Encode(placeholder), off); err != nil {
		return 0, err
	}
	h.live++
	return off, h.writeHeader()
}

// Free releases the slot at off and pushes it onto the free list. O(1).
// The visit data is wiped, so a freed record cannot reappear.
func (h *File) Free(off int64) error {
	cur, err := h.Read(off)
	if err != nil {
		return err
	}
	if !cur.Live {
		return fmt.Errorf("%w: %d", ErrNotLive, off)
	}

	free := record.Record{Live: false, Links: record.NoLinks()}
	free.Links.NextSpecies = h.freeHead
	if _, err := h.f.WriteAt(record.Encode(free), off); err != nil {
		return err
	}
	h.freeHead = off
	h.live--
	return h.writeHeader()
}

// Read returns the record at off, live or free. O(1): one ReadAt.
func (h *File) Read(off int64) (record.Record, error) {
	if err := h.checkOffset(off); err != nil {
		return record.Record{}, err
	}
	b := make([]byte, record.RecordSize)
	if _, err := h.f.ReadAt(b, off); err != nil {
		return record.Record{}, err
	}
	return record.Decode(b)
}

// Write overwrites the live record at off in place. O(1): one WriteAt.
// Use Free to delete a record.
func (h *File) Write(off int64, r record.Record) error {
	if !r.Live {
		return fmt.Errorf("%w: use Free to delete a record", ErrNotLive)
	}
	cur, err := h.Read(off)
	if err != nil {
		return err
	}
	if !cur.Live {
		return fmt.Errorf("%w: %d, call Alloc first", ErrNotLive, off)
	}
	_, err = h.f.WriteAt(record.Encode(r), off)
	return err
}

// Scan calls fn for every live record in file order and stops early if fn
// returns false. It reads the file sequentially in chunks. O(slots).
//
// This is the only full pass over the data: it is used to show the table
// on screen, to export and to search by a field that has no index.
func (h *File) Scan(fn func(off int64, r record.Record) bool) error {
	if h.f == nil {
		return ErrClosed
	}
	buf := make([]byte, scanChunk*record.RecordSize)
	for start := uint64(0); start < h.slots; start += scanChunk {
		n := min(uint64(scanChunk), h.slots-start)
		chunk := buf[:n*record.RecordSize]
		if _, err := h.f.ReadAt(chunk, slotOffset(start)); err != nil {
			return err
		}
		for j := uint64(0); j < n; j++ {
			r, err := record.Decode(chunk[j*record.RecordSize : (j+1)*record.RecordSize])
			if err != nil {
				return err
			}
			if r.Live && !fn(slotOffset(start+j), r) {
				return nil
			}
		}
	}
	return nil
}
