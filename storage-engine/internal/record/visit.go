// Package record defines the Visit model and its fixed-size binary format.
//
// The package knows nothing about files: it only validates visits and turns
// them into exactly RecordSize bytes and back.
package record

import (
	"errors"
	"fmt"
	"time"
)

// Status is the state of a visit. The zero value is intentionally invalid,
// so a visit with a forgotten status fails validation.
type Status uint8

// Visit statuses.
const (
	StatusScheduled Status = iota + 1
	StatusVisited
	StatusCancelled
	StatusNoShow
)

// ErrUnknownStatus is returned by ParseStatus for an unknown status name.
var ErrUnknownStatus = errors.New("unknown status")

var statusNames = map[Status]string{
	StatusScheduled: "Запланирован",
	StatusVisited:   "Состоялся",
	StatusCancelled: "Отменён",
	StatusNoShow:    "Не пришёл",
}

// String returns the human-readable (Russian) name of the status.
func (s Status) String() string {
	if name, ok := statusNames[s]; ok {
		return name
	}
	return fmt.Sprintf("Status(%d)", uint8(s))
}

// Valid reports whether s is one of the known statuses.
func (s Status) Valid() bool {
	_, ok := statusNames[s]
	return ok
}

// AllStatuses returns every valid status in display order.
func AllStatuses() []Status {
	return []Status{StatusScheduled, StatusVisited, StatusCancelled, StatusNoShow}
}

// ParseStatus converts a name produced by Status.String back to a Status.
func ParseStatus(name string) (Status, error) {
	for _, s := range AllStatuses() {
		if s.String() == name {
			return s, nil
		}
	}
	return 0, fmt.Errorf("%w: %q", ErrUnknownStatus, name)
}

// Visit is one appointment at the clinic: what the user sees and edits.
type Visit struct {
	ID      int32 // key field
	PetName string
	Species string // indexed field
	Doctor  string // indexed field
	At      time.Time
	Status  Status
	Price   float64
}

// Links are the storage-level pointers of the doubly linked chains used by
// the secondary indexes (species and doctor). NilOffset means "no record".
type Links struct {
	PrevSpecies int64
	NextSpecies int64
	PrevDoctor  int64
	NextDoctor  int64
}

// NoLinks returns Links with every pointer set to NilOffset.
func NoLinks() Links {
	return Links{
		PrevSpecies: NilOffset,
		NextSpecies: NilOffset,
		PrevDoctor:  NilOffset,
		NextDoctor:  NilOffset,
	}
}

// Record is a Visit as it is stored on disk: the visit plus the live flag
// and the service links.
type Record struct {
	Live  bool
	Visit Visit
	Links Links
}
