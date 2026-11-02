// Package db is the database facade: it ties the heap file with records and
// three on-disk hash indexes into one table of visits.
//
// A database is a directory with four files:
//
//	visits.dat   records (package heap)
//	key.idx      visit_id -> record offset
//	species.idx  species  -> offset of the first record with that species
//	doctor.idx   doctor   -> offset of the first record with that doctor
//
// Records with the same species (or doctor) form a doubly linked chain
// through the Prev/Next links stored inside the records, so finding or
// deleting all of them costs O(1 + k) and unlinking one record costs O(1).
//
// Every operation reads and writes only the bytes it needs. Nothing is
// loaded into memory except by Scan, which the GUI uses to draw the table.
package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sseusti/VetCare/storage-engine/internal/hashfile"
	"github.com/sseusti/VetCare/storage-engine/internal/heap"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// File names inside a database directory.
const (
	FileData    = "visits.dat"
	FileKey     = "key.idx"
	FileSpecies = "species.idx"
	FileDoctor  = "doctor.idx"
)

// Files lists every file of a database, data file first.
var Files = []string{FileData, FileKey, FileSpecies, FileDoctor}

var (
	// ErrExists is returned by Create when the directory already holds a database.
	ErrExists = errors.New("db: database already exists")
	// ErrNotDB is returned by Open when the directory is not a database.
	ErrNotDB = errors.New("db: not a database directory")
	// ErrCorrupt is returned when the files disagree with each other.
	ErrCorrupt = errors.New("db: database files are inconsistent")
	// ErrDuplicateKey is returned when a visit ID is already taken.
	ErrDuplicateKey = errors.New("db: visit with this ID already exists")
	// ErrNotFound is returned when no visit has the given ID.
	ErrNotFound = errors.New("db: visit not found")
	// ErrBadValue is returned when a search value cannot be parsed.
	ErrBadValue = errors.New("db: bad search value")
	// ErrClosed is returned when the database is used after Close.
	ErrClosed = errors.New("db: database is closed")
)

// DB is an open database. It is not safe for concurrent use.
type DB struct {
	dir     string
	heap    *heap.File
	key     *hashfile.File
	species *hashfile.File
	doctor  *hashfile.File
}

// Create creates a new empty database in dir, creating dir if needed.
func Create(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, FileData)); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrExists, dir)
	}

	d := &DB{dir: dir}
	var err error
	if d.heap, err = heap.Create(d.path(FileData)); err != nil {
		return nil, err
	}
	if d.key, err = hashfile.Create(d.path(FileKey), 4, 0); err != nil {
		d.closeFiles()
		return nil, err
	}
	if d.species, err = hashfile.Create(d.path(FileSpecies), record.SpeciesSize, 0); err != nil {
		d.closeFiles()
		return nil, err
	}
	if d.doctor, err = hashfile.Create(d.path(FileDoctor), record.DoctorSize, 0); err != nil {
		d.closeFiles()
		return nil, err
	}
	return d, nil
}

// Open opens an existing database in dir.
func Open(dir string) (*DB, error) {
	d := &DB{dir: dir}
	if err := d.openFiles(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DB) openFiles() error {
	var err error
	fail := func(e error) error {
		d.closeFiles()
		if errors.Is(e, os.ErrNotExist) {
			return fmt.Errorf("%w: %v", ErrNotDB, e)
		}
		return e
	}
	if d.heap, err = heap.Open(d.path(FileData)); err != nil {
		return fail(err)
	}
	if d.key, err = hashfile.Open(d.path(FileKey)); err != nil {
		return fail(err)
	}
	if d.species, err = hashfile.Open(d.path(FileSpecies)); err != nil {
		return fail(err)
	}
	if d.doctor, err = hashfile.Open(d.path(FileDoctor)); err != nil {
		return fail(err)
	}
	if d.heap.Len() != d.key.Len() {
		return fail(fmt.Errorf("%w: %d records, %d keys", ErrCorrupt, d.heap.Len(), d.key.Len()))
	}
	return nil
}

// Dir returns the database directory.
func (d *DB) Dir() string { return d.dir }

// Len returns the number of visits.
func (d *DB) Len() uint64 {
	if d.heap == nil {
		return 0
	}
	return d.heap.Len()
}

// Sync flushes every file to disk. Changes are written immediately, so this
// is what "save" means here.
func (d *DB) Sync() error {
	if d.heap == nil {
		return ErrClosed
	}
	for _, s := range []interface{ Sync() error }{d.heap, d.key, d.species, d.doctor} {
		if err := s.Sync(); err != nil {
			return err
		}
	}
	return nil
}

// Clear removes every visit and shrinks all files.
func (d *DB) Clear() error {
	if d.heap == nil {
		return ErrClosed
	}
	if err := d.heap.Clear(); err != nil {
		return err
	}
	for _, h := range []*hashfile.File{d.key, d.species, d.doctor} {
		if err := h.Clear(); err != nil {
			return err
		}
	}
	return nil
}

// Close closes all files.
func (d *DB) Close() error {
	if d.heap == nil {
		return ErrClosed
	}
	return d.closeFiles()
}

// Drop closes the database and deletes its files. The directory is removed
// too if nothing else is left in it.
func (d *DB) Drop() error {
	if d.heap != nil {
		d.closeFiles()
	}
	for _, name := range Files {
		if err := os.Remove(d.path(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	os.Remove(d.dir) // fails harmlessly if the directory is not empty
	return nil
}

// Scan calls fn for every visit in file order until fn returns false. It is
// the only full pass: used to draw the table and to export.
func (d *DB) Scan(fn func(v record.Visit) bool) error {
	if d.heap == nil {
		return ErrClosed
	}
	return d.heap.Scan(func(_ int64, r record.Record) bool { return fn(r.Visit) })
}

func (d *DB) path(name string) string { return filepath.Join(d.dir, name) }

// closeFiles closes whatever is open and marks the database closed.
func (d *DB) closeFiles() error {
	var first error
	if d.heap != nil {
		first = d.heap.Close()
	}
	for _, h := range []*hashfile.File{d.key, d.species, d.doctor} {
		if h != nil {
			if err := h.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	d.heap, d.key, d.species, d.doctor = nil, nil, nil, nil
	return first
}
