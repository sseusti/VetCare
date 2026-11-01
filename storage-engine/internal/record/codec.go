package record

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf8"
)

// Maximum sizes of string fields, in bytes (not characters): a Cyrillic
// letter takes 2 bytes in UTF-8.
const (
	PetNameSize = 48
	SpeciesSize = 32
	DoctorSize  = 64
)

// NilOffset marks an empty link. Offset 0 is a real position in a file,
// so it cannot mean "nothing".
const NilOffset int64 = -1

// Byte layout of a record. All numbers are little-endian.
const (
	offFlag        = 0                        // uint8: 1 = live, 0 = free
	offID          = offFlag + 1              // int32
	offPetName     = offID + 4                // [PetNameSize]byte
	offSpecies     = offPetName + PetNameSize // [SpeciesSize]byte
	offDoctor      = offSpecies + SpeciesSize // [DoctorSize]byte
	offAt          = offDoctor + DoctorSize   // int64, unix seconds
	offStatus      = offAt + 8                // uint8
	offPrice       = offStatus + 1            // float64 bits
	offPrevSpecies = offPrice + 8             // int64
	offNextSpecies = offPrevSpecies + 8       // int64
	offPrevDoctor  = offNextSpecies + 8       // int64
	offNextDoctor  = offPrevDoctor + 8        // int64
	RecordSize     = offNextDoctor + 8        // 198 bytes
)

const flagLive = 1

// ErrBadLength is returned by Decode when the buffer is not RecordSize long.
var ErrBadLength = errors.New("record: bad buffer length")

// Encode serializes r into exactly RecordSize bytes. Strings longer than
// their field are cut on a UTF-8 character boundary; call Validate first to
// reject them instead.
func Encode(r Record) []byte {
	b := make([]byte, RecordSize)
	if r.Live {
		b[offFlag] = flagLive
	}
	v := r.Visit
	binary.LittleEndian.PutUint32(b[offID:], uint32(v.ID))
	putString(b[offPetName:offSpecies], v.PetName)
	putString(b[offSpecies:offDoctor], v.Species)
	putString(b[offDoctor:offAt], v.Doctor)
	binary.LittleEndian.PutUint64(b[offAt:], uint64(v.At.Unix()))
	b[offStatus] = byte(v.Status)
	binary.LittleEndian.PutUint64(b[offPrice:], math.Float64bits(v.Price))
	putOffset(b[offPrevSpecies:], r.Links.PrevSpecies)
	putOffset(b[offNextSpecies:], r.Links.NextSpecies)
	putOffset(b[offPrevDoctor:], r.Links.PrevDoctor)
	putOffset(b[offNextDoctor:], r.Links.NextDoctor)
	return b
}

// Decode parses a buffer produced by Encode. Time is restored with second
// precision in the local time zone, so compare it with time.Time.Equal.
func Decode(b []byte) (Record, error) {
	if len(b) != RecordSize {
		return Record{}, fmt.Errorf("%w: got %d, want %d", ErrBadLength, len(b), RecordSize)
	}
	return Record{
		Live: b[offFlag] == flagLive,
		Visit: Visit{
			ID:      int32(binary.LittleEndian.Uint32(b[offID:])),
			PetName: getString(b[offPetName:offSpecies]),
			Species: getString(b[offSpecies:offDoctor]),
			Doctor:  getString(b[offDoctor:offAt]),
			At:      time.Unix(int64(binary.LittleEndian.Uint64(b[offAt:])), 0),
			Status:  Status(b[offStatus]),
			Price:   math.Float64frombits(binary.LittleEndian.Uint64(b[offPrice:])),
		},
		Links: Links{
			PrevSpecies: getOffset(b[offPrevSpecies:]),
			NextSpecies: getOffset(b[offNextSpecies:]),
			PrevDoctor:  getOffset(b[offPrevDoctor:]),
			NextDoctor:  getOffset(b[offNextDoctor:]),
		},
	}, nil
}

// truncateUTF8 cuts s to at most max bytes without splitting a character.
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// putString writes s into dst and pads the rest with zero bytes.
func putString(dst []byte, s string) {
	n := copy(dst, truncateUTF8(s, len(dst)))
	clear(dst[n:])
}

// getString reads a zero-padded string.
func getString(src []byte) string {
	return string(bytes.TrimRight(src, "\x00"))
}

func putOffset(dst []byte, off int64) {
	binary.LittleEndian.PutUint64(dst, uint64(off))
}

func getOffset(src []byte) int64 {
	return int64(binary.LittleEndian.Uint64(src))
}
