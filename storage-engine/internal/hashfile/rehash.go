package hashfile

import (
	"os"
)

// rehashChunk is how many slots are read from the old file at once.
const rehashChunk = 4096

// rehash rebuilds the table into a fresh file and swaps it in.
//
// If more than half of the slots hold live keys, the new table is twice as
// big; otherwise it keeps its size and the rebuild only drops tombstones.
// The new file is written next to the old one and replaces it with an
// atomic rename, so a crash leaves either the old or the new table.
//
// Cost: O(capacity), but it happens after Θ(capacity) inserts, so the
// amortized cost of an insert stays O(1).
func (h *File) rehash() error {
	newCap := h.capacity
	if (h.count+1)*2 > h.capacity {
		newCap *= 2
	}

	tmpPath := h.path + ".rehash"
	nh, err := Create(tmpPath, h.keySize, newCap)
	if err != nil {
		return err
	}
	if err := h.copyLiveSlots(nh); err != nil {
		nh.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := nh.writeHeader(); err != nil {
		nh.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := nh.f.Sync(); err != nil {
		nh.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, h.path); err != nil {
		nh.Close()
		os.Remove(tmpPath)
		return err
	}

	// The renamed file is now the table: take over nh's handle.
	h.f.Close()
	h.f = nh.f
	h.capacity, h.count, h.tombstones = nh.capacity, nh.count, 0
	return nil
}

// copyLiveSlots reads the old table sequentially, in chunks, and inserts
// every live key into dst. dst is fresh, so there are no tombstones and no
// duplicates: each key goes to the first empty slot of its probe chain.
func (h *File) copyLiveSlots(dst *File) error {
	buf := make([]byte, rehashChunk*h.slotSize)
	for start := uint64(0); start < h.capacity; start += rehashChunk {
		n := min(uint64(rehashChunk), h.capacity-start)
		chunk := buf[:n*uint64(h.slotSize)]
		if _, err := h.f.ReadAt(chunk, h.slotOffset(start)); err != nil {
			return err
		}
		for j := uint64(0); j < n; j++ {
			state, key, val := decodeSlot(chunk[j*uint64(h.slotSize):], h.keySize)
			if state != stateUsed {
				continue
			}
			i := hashKey(key) & (dst.capacity - 1)
			for {
				s, _, _, err := dst.readSlot(i)
				if err != nil {
					return err
				}
				if s == stateEmpty {
					break
				}
				i = dst.next(i)
			}
			if err := dst.writeSlot(i, stateUsed, key, val); err != nil {
				return err
			}
			dst.count++
		}
	}
	return nil
}
