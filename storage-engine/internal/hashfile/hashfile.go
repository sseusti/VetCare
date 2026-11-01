// Package hashfile implements a hash table that lives in a file: open
// addressing with linear probing, tombstones on delete and rehash into a
// bigger file when the table gets too full.
//
// Keys are byte strings of a fixed size (shorter keys are zero-padded),
// values are int64. The database uses it three times: visit_id -> record
// offset (key.idx) and species / doctor -> head of a chain (species.idx,
// doctor.idx).
//
// File layout:
//
//	header, 64 bytes:
//	  0  magic "VETHASH1"
//	  8  keySize    uint32
//	  12 capacity   uint64 (power of two)
//	  20 count      uint64 (used slots)
//	  28 tombstones uint64 (deleted slots)
//	slots, capacity × (1 + keySize + 8) bytes:
//	  state uint8 (0 empty, 1 used, 2 deleted), key, value int64
//
// All numbers are little-endian. A zero-filled slot is empty, so a new
// table is just a header plus zeros.
package hashfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	headerSize  = 64
	minCapacity = 16

	// The table is rebuilt when (count + tombstones) / capacity would
	// exceed maxLoadNum / maxLoadDen = 0.7.
	maxLoadNum = 7
	maxLoadDen = 10
)

const (
	stateEmpty   byte = 0
	stateUsed    byte = 1
	stateDeleted byte = 2
)

var magic = [8]byte{'V', 'E', 'T', 'H', 'A', 'S', 'H', '1'}

var (
	// ErrKeySize is returned for a key longer than the table's key size.
	ErrKeySize = errors.New("hashfile: key is longer than key size")
	// ErrBadFile is returned when a file is not a hash table or is damaged.
	ErrBadFile = errors.New("hashfile: not a hash file or damaged")
	// ErrClosed is returned when the table is used after Close.
	ErrClosed = errors.New("hashfile: table is closed")
)

// File is an open on-disk hash table. It is not safe for concurrent use.
type File struct {
	f          *os.File
	path       string
	keySize    int
	slotSize   int
	capacity   uint64
	count      uint64
	tombstones uint64
}

// Create creates (or truncates) a table at path. capacity is rounded up to a
// power of two, at least 16.
func Create(path string, keySize int, capacity uint64) (*File, error) {
	if keySize <= 0 {
		return nil, fmt.Errorf("hashfile: key size must be positive, got %d", keySize)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	h := &File{
		f:        f,
		path:     path,
		keySize:  keySize,
		slotSize: 1 + keySize + 8,
		capacity: roundCapacity(capacity),
	}
	if err := h.reset(); err != nil {
		f.Close()
		return nil, err
	}
	return h, nil
}

// Open opens an existing table and checks its header.
func Open(path string) (*File, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	h := &File{f: f, path: path}
	if err := h.readHeader(); err != nil {
		f.Close()
		return nil, err
	}
	return h, nil
}

// Len returns the number of keys in the table.
func (h *File) Len() uint64 { return h.count }

// Capacity returns the number of slots.
func (h *File) Capacity() uint64 { return h.capacity }

// KeySize returns the fixed key size in bytes.
func (h *File) KeySize() int { return h.keySize }

// Clear removes every key and shrinks the table back to the minimum size.
func (h *File) Clear() error {
	if h.f == nil {
		return ErrClosed
	}
	h.capacity = minCapacity
	return h.reset()
}

// Sync flushes the file to disk.
func (h *File) Sync() error {
	if h.f == nil {
		return ErrClosed
	}
	return h.f.Sync()
}

// Close closes the file. Using the table afterwards returns ErrClosed.
func (h *File) Close() error {
	if h.f == nil {
		return ErrClosed
	}
	err := h.f.Close()
	h.f = nil
	return err
}

// reset makes the file an empty table of h.capacity slots. Truncate fills
// the new space with zeros, and a zero slot is empty.
func (h *File) reset() error {
	h.count, h.tombstones = 0, 0
	if err := h.f.Truncate(0); err != nil {
		return err
	}
	if err := h.f.Truncate(h.fileSize()); err != nil {
		return err
	}
	return h.writeHeader()
}

func (h *File) fileSize() int64 {
	return headerSize + int64(h.capacity)*int64(h.slotSize)
}

func (h *File) writeHeader() error {
	var b [headerSize]byte
	copy(b[0:8], magic[:])
	binary.LittleEndian.PutUint32(b[8:], uint32(h.keySize))
	binary.LittleEndian.PutUint64(b[12:], h.capacity)
	binary.LittleEndian.PutUint64(b[20:], h.count)
	binary.LittleEndian.PutUint64(b[28:], h.tombstones)
	_, err := h.f.WriteAt(b[:], 0)
	return err
}

func (h *File) readHeader() error {
	var b [headerSize]byte
	if _, err := h.f.ReadAt(b[:], 0); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: file is shorter than the header", ErrBadFile)
		}
		return err
	}
	if [8]byte(b[0:8]) != magic {
		return fmt.Errorf("%w: wrong magic", ErrBadFile)
	}
	h.keySize = int(binary.LittleEndian.Uint32(b[8:]))
	h.slotSize = 1 + h.keySize + 8
	h.capacity = binary.LittleEndian.Uint64(b[12:])
	h.count = binary.LittleEndian.Uint64(b[20:])
	h.tombstones = binary.LittleEndian.Uint64(b[28:])

	switch {
	case h.keySize <= 0:
		return fmt.Errorf("%w: key size %d", ErrBadFile, h.keySize)
	case h.capacity < minCapacity || h.capacity&(h.capacity-1) != 0:
		return fmt.Errorf("%w: capacity %d is not a power of two >= %d", ErrBadFile, h.capacity, minCapacity)
	case h.count+h.tombstones > h.capacity:
		return fmt.Errorf("%w: more keys than slots", ErrBadFile)
	}
	st, err := h.f.Stat()
	if err != nil {
		return err
	}
	if st.Size() != h.fileSize() {
		return fmt.Errorf("%w: size %d, want %d", ErrBadFile, st.Size(), h.fileSize())
	}
	return nil
}

func roundCapacity(n uint64) uint64 {
	c := uint64(minCapacity)
	for c < n {
		c <<= 1
	}
	return c
}
