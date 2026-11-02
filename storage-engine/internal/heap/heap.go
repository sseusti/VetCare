// Package heap stores visit records in one file of fixed-size slots.
//
// A record lives at a stable byte offset for its whole life, so indexes can
// point at it. Deleted slots are chained into a free list inside the file
// and reused by later inserts, so the file never has to be shifted.
//
// File layout:
//
//	header, 64 bytes:
//	  0  magic "VETHEAP1"
//	  8  recordSize uint32 (must equal record.RecordSize)
//	  12 slots      uint64 (slots in the file, live or free)
//	  20 live       uint64 (live records)
//	  28 freeHead   int64  (offset of the first free slot, -1 if none)
//	slots: slots × record.RecordSize bytes, slot i at 64 + i*RecordSize
//
// A free slot is a record with the live flag cleared; its Links.NextSpecies
// holds the offset of the next free slot. All numbers are little-endian.
package heap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// HeaderSize is the size of the file header; the first slot starts here.
const HeaderSize = 64

var magic = [8]byte{'V', 'E', 'T', 'H', 'E', 'A', 'P', '1'}

var (
	// ErrBadFile is returned when a file is not a heap file or is damaged.
	ErrBadFile = errors.New("heap: not a heap file or damaged")
	// ErrBadOffset is returned for an offset that is not the start of a slot.
	ErrBadOffset = errors.New("heap: offset is not a slot")
	// ErrNotLive is returned when freeing or updating a free slot.
	ErrNotLive = errors.New("heap: slot is not live")
	// ErrClosed is returned when the file is used after Close.
	ErrClosed = errors.New("heap: file is closed")
)

// File is an open heap file. It is not safe for concurrent use.
type File struct {
	f        *os.File
	slots    uint64
	live     uint64
	freeHead int64
}

// Create creates (or truncates) an empty heap file at path.
func Create(path string) (*File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	h := &File{f: f}
	if err := h.reset(); err != nil {
		f.Close()
		return nil, err
	}
	return h, nil
}

// Open opens an existing heap file and checks its header.
func Open(path string) (*File, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	h := &File{f: f}
	if err := h.readHeader(); err != nil {
		f.Close()
		return nil, err
	}
	return h, nil
}

// Len returns the number of live records.
func (h *File) Len() uint64 { return h.live }

// Slots returns the number of slots in the file, live or free.
func (h *File) Slots() uint64 { return h.slots }

// Clear removes every record and shrinks the file to its header.
func (h *File) Clear() error {
	if h.f == nil {
		return ErrClosed
	}
	return h.reset()
}

// Sync flushes the file to disk. Every change is written immediately, so
// this is what "save" means for the database.
func (h *File) Sync() error {
	if h.f == nil {
		return ErrClosed
	}
	return h.f.Sync()
}

// Close closes the file. Using it afterwards returns ErrClosed.
func (h *File) Close() error {
	if h.f == nil {
		return ErrClosed
	}
	err := h.f.Close()
	h.f = nil
	return err
}

func (h *File) reset() error {
	h.slots, h.live, h.freeHead = 0, 0, record.NilOffset
	if err := h.f.Truncate(HeaderSize); err != nil {
		return err
	}
	return h.writeHeader()
}

// slotOffset returns the byte offset of slot i.
func slotOffset(i uint64) int64 {
	return HeaderSize + int64(i)*record.RecordSize
}

// checkOffset verifies that off is the start of an existing slot.
func (h *File) checkOffset(off int64) error {
	if h.f == nil {
		return ErrClosed
	}
	if off < HeaderSize || (off-HeaderSize)%record.RecordSize != 0 || off >= slotOffset(h.slots) {
		return fmt.Errorf("%w: %d", ErrBadOffset, off)
	}
	return nil
}

func (h *File) writeHeader() error {
	var b [HeaderSize]byte
	copy(b[0:8], magic[:])
	binary.LittleEndian.PutUint32(b[8:], record.RecordSize)
	binary.LittleEndian.PutUint64(b[12:], h.slots)
	binary.LittleEndian.PutUint64(b[20:], h.live)
	binary.LittleEndian.PutUint64(b[28:], uint64(h.freeHead))
	_, err := h.f.WriteAt(b[:], 0)
	return err
}

func (h *File) readHeader() error {
	var b [HeaderSize]byte
	if _, err := h.f.ReadAt(b[:], 0); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: file is shorter than the header", ErrBadFile)
		}
		return err
	}
	if [8]byte(b[0:8]) != magic {
		return fmt.Errorf("%w: wrong magic", ErrBadFile)
	}
	if rs := binary.LittleEndian.Uint32(b[8:]); rs != record.RecordSize {
		return fmt.Errorf("%w: record size %d, this build uses %d", ErrBadFile, rs, record.RecordSize)
	}
	h.slots = binary.LittleEndian.Uint64(b[12:])
	h.live = binary.LittleEndian.Uint64(b[20:])
	h.freeHead = int64(binary.LittleEndian.Uint64(b[28:]))

	if h.live > h.slots {
		return fmt.Errorf("%w: %d live records in %d slots", ErrBadFile, h.live, h.slots)
	}
	st, err := h.f.Stat()
	if err != nil {
		return err
	}
	if want := slotOffset(h.slots); st.Size() != want {
		return fmt.Errorf("%w: size %d, want %d", ErrBadFile, st.Size(), want)
	}
	if h.freeHead != record.NilOffset {
		if err := h.checkOffset(h.freeHead); err != nil {
			return fmt.Errorf("%w: free list head: %v", ErrBadFile, err)
		}
	}
	return nil
}
