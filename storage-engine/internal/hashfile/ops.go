package hashfile

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const noSlot = ^uint64(0)

// Int32Key encodes an int32 as a 4-byte little-endian key.
func Int32Key(v int32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return b
}

// Get returns the value stored under key. O(1) on average: one hash and a
// short run of probes, each probe is one ReadAt.
func (h *File) Get(key []byte) (val int64, ok bool, err error) {
	k, err := h.padKey(key)
	if err != nil {
		return 0, false, err
	}
	p, err := h.probe(k)
	if err != nil || !p.found {
		return 0, false, err
	}
	return p.val, true, nil
}

// Put stores val under key, replacing an existing value.
func (h *File) Put(key []byte, val int64) error {
	k, err := h.padKey(key)
	if err != nil {
		return err
	}
	p, err := h.probe(k)
	if err != nil {
		return err
	}
	if p.found {
		return h.writeSlot(p.idx, stateUsed, k, val)
	}

	if (h.count+h.tombstones+1)*maxLoadDen > h.capacity*maxLoadNum {
		if err := h.rehash(); err != nil {
			return err
		}
		if p, err = h.probe(k); err != nil {
			return err
		}
	}

	if err := h.writeSlot(p.free, stateUsed, k, val); err != nil {
		return err
	}
	if p.freeWasDeleted {
		h.tombstones--
	}
	h.count++
	return h.writeHeader()
}

// Delete removes key and reports whether it was present.
//
// Normally the slot becomes a tombstone: an empty slot there would stop
// later lookups of keys that probed past it. But if the next slot is empty,
// no probe chain goes through this slot, so it can safely become empty.
func (h *File) Delete(key []byte) (bool, error) {
	k, err := h.padKey(key)
	if err != nil {
		return false, err
	}
	p, err := h.probe(k)
	if err != nil || !p.found {
		return false, err
	}

	next, _, _, err := h.readSlot(h.next(p.idx))
	if err != nil {
		return false, err
	}
	state := stateDeleted
	if next == stateEmpty {
		state = stateEmpty
	} else {
		h.tombstones++
	}
	if err := h.writeSlot(p.idx, state, nil, 0); err != nil {
		return false, err
	}
	h.count--
	return true, h.writeHeader()
}

// probeResult is what a walk along the probe chain of a key found.
type probeResult struct {
	found          bool
	idx            uint64 // slot of the key, if found
	val            int64
	free           uint64 // first slot where the key can be inserted
	freeWasDeleted bool   // free is a tombstone, not an empty slot
}

// probe walks from the key's home slot until it finds the key or an empty
// slot. On the way it remembers the first tombstone, so an insert reuses it.
func (h *File) probe(k []byte) (probeResult, error) {
	r := probeResult{free: noSlot}
	i := hashKey(k) & (h.capacity - 1)
	for n := uint64(0); n < h.capacity; n++ {
		state, key, val, err := h.readSlot(i)
		if err != nil {
			return r, err
		}
		switch state {
		case stateEmpty:
			if r.free == noSlot {
				r.free = i
			}
			return r, nil
		case stateDeleted:
			if r.free == noSlot {
				r.free, r.freeWasDeleted = i, true
			}
		case stateUsed:
			if bytes.Equal(key, k) {
				r.found, r.idx, r.val = true, i, val
				return r, nil
			}
		default:
			return r, fmt.Errorf("%w: slot %d has state %d", ErrBadFile, i, state)
		}
		i = h.next(i)
	}
	// Unreachable while the load factor stays below 1: there is always an
	// empty slot to stop at.
	return r, nil
}

func (h *File) next(i uint64) uint64 { return (i + 1) & (h.capacity - 1) }

func (h *File) slotOffset(i uint64) int64 {
	return headerSize + int64(i)*int64(h.slotSize)
}

func (h *File) readSlot(i uint64) (state byte, key []byte, val int64, err error) {
	if h.f == nil {
		return 0, nil, 0, ErrClosed
	}
	b := make([]byte, h.slotSize)
	if _, err := h.f.ReadAt(b, h.slotOffset(i)); err != nil {
		return 0, nil, 0, err
	}
	state, key, val = decodeSlot(b, h.keySize)
	return state, key, val, nil
}

func (h *File) writeSlot(i uint64, state byte, key []byte, val int64) error {
	if h.f == nil {
		return ErrClosed
	}
	b := make([]byte, h.slotSize)
	b[0] = state
	copy(b[1:1+h.keySize], key)
	binary.LittleEndian.PutUint64(b[1+h.keySize:], uint64(val))
	_, err := h.f.WriteAt(b, h.slotOffset(i))
	return err
}

func decodeSlot(b []byte, keySize int) (state byte, key []byte, val int64) {
	return b[0], b[1 : 1+keySize], int64(binary.LittleEndian.Uint64(b[1+keySize:]))
}

// padKey copies key into a keySize buffer, padding it with zeros.
func (h *File) padKey(key []byte) ([]byte, error) {
	if h.f == nil {
		return nil, ErrClosed
	}
	if len(key) > h.keySize {
		return nil, fmt.Errorf("%w: %d > %d", ErrKeySize, len(key), h.keySize)
	}
	k := make([]byte, h.keySize)
	copy(k, key)
	return k, nil
}

// hashKey is 64-bit FNV-1a: simple, fast and spreads both sequential
// integers and short strings well.
func hashKey(k []byte) uint64 {
	h := uint64(14695981039346656037)
	for _, b := range k {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return h
}
