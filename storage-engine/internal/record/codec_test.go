package record

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func sampleVisit() Visit {
	return Visit{
		ID:      17,
		PetName: "Барсик",
		Species: "Кошка",
		Doctor:  "Иванова А. П.",
		At:      time.Date(2026, 11, 2, 14, 30, 0, 0, time.UTC),
		Status:  StatusScheduled,
		Price:   1500.5,
	}
}

func assertRecordEqual(t *testing.T, got, want Record) {
	t.Helper()
	if !got.Visit.At.Equal(want.Visit.At) {
		t.Errorf("At = %v, want %v", got.Visit.At, want.Visit.At)
	}
	got.Visit.At, want.Visit.At = time.Time{}, time.Time{}
	if got != want {
		t.Errorf("record mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestRecordSize(t *testing.T) {
	if RecordSize != 198 {
		t.Fatalf("RecordSize = %d, want 198", RecordSize)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	latin := sampleVisit()
	latin.PetName, latin.Species, latin.Doctor = "Rex", "Dog", "Dr. Smith"

	free := sampleVisit()

	maxID := sampleVisit()
	maxID.ID = 1<<31 - 1
	maxID.Price = 0

	tests := []struct {
		name string
		rec  Record
	}{
		{"cyrillic, no links", Record{Live: true, Visit: sampleVisit(), Links: NoLinks()}},
		{"latin", Record{Live: true, Visit: latin, Links: NoLinks()}},
		{"free slot", Record{Live: false, Visit: free, Links: NoLinks()}},
		{"real links", Record{Live: true, Visit: sampleVisit(), Links: Links{0, 198, 396, 1 << 40}}},
		{"max id, zero price", Record{Live: true, Visit: maxID, Links: NoLinks()}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := Encode(tc.rec)
			if len(b) != RecordSize {
				t.Fatalf("len(Encode) = %d, want %d", len(b), RecordSize)
			}
			got, err := Decode(b)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			assertRecordEqual(t, got, tc.rec)
		})
	}
}

func TestEncodeTruncatesOnRuneBoundary(t *testing.T) {
	v := sampleVisit()
	v.PetName = strings.Repeat("Ж", 30) // 60 bytes into a 48-byte field
	v.Species = "Кошка" + strings.Repeat("я", 20)

	got, err := Decode(Encode(Record{Live: true, Visit: v}))
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Repeat("Ж", 24); got.Visit.PetName != want {
		t.Errorf("PetName = %q, want %q", got.Visit.PetName, want)
	}
	if !utf8.ValidString(got.Visit.Species) || len(got.Visit.Species) > SpeciesSize {
		t.Errorf("Species = %q: must be valid UTF-8 within %d bytes", got.Visit.Species, SpeciesSize)
	}
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"Барсик", 12, "Барсик"}, // exactly fits
		{"Барсик", 11, "Барси"},  // would split "к"
		{"Барсик", 5, "Ба"},      // "Бар" needs 6 bytes
		{"abc", 10, "abc"},
		{"Б", 1, ""},
		{"", 4, ""},
	}
	for _, tc := range tests {
		if got := truncateUTF8(tc.s, tc.max); got != tc.want {
			t.Errorf("truncateUTF8(%q, %d) = %q, want %q", tc.s, tc.max, got, tc.want)
		}
	}
}

func TestDecodeBadLength(t *testing.T) {
	for _, n := range []int{0, RecordSize - 1, RecordSize + 1} {
		if _, err := Decode(make([]byte, n)); !errors.Is(err, ErrBadLength) {
			t.Errorf("Decode(len %d): want ErrBadLength, got %v", n, err)
		}
	}
}

func TestDecodeZeroBufferIsFreeSlot(t *testing.T) {
	got, err := Decode(make([]byte, RecordSize))
	if err != nil {
		t.Fatal(err)
	}
	if got.Live {
		t.Error("zeroed slot must decode as not live")
	}
}
