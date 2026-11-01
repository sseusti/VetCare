package record

import (
	"errors"
	"testing"
)

func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusScheduled, "Запланирован"},
		{StatusVisited, "Состоялся"},
		{StatusCancelled, "Отменён"},
		{StatusNoShow, "Не пришёл"},
		{0, "Status(0)"},
		{42, "Status(42)"},
	}
	for _, tc := range tests {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("Status(%d).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestStatusValid(t *testing.T) {
	for _, s := range AllStatuses() {
		if !s.Valid() {
			t.Errorf("%v must be valid", s)
		}
	}
	if Status(0).Valid() {
		t.Error("zero status must be invalid")
	}
	if Status(5).Valid() {
		t.Error("Status(5) must be invalid")
	}
}

func TestParseStatusRoundTrip(t *testing.T) {
	for _, s := range AllStatuses() {
		got, err := ParseStatus(s.String())
		if err != nil {
			t.Fatalf("ParseStatus(%q): %v", s.String(), err)
		}
		if got != s {
			t.Errorf("ParseStatus(%q) = %v, want %v", s.String(), got, s)
		}
	}
}

func TestParseStatusUnknown(t *testing.T) {
	_, err := ParseStatus("Улетел")
	if !errors.Is(err, ErrUnknownStatus) {
		t.Fatalf("want ErrUnknownStatus, got %v", err)
	}
}

func TestNoLinks(t *testing.T) {
	l := NoLinks()
	for name, v := range map[string]int64{
		"PrevSpecies": l.PrevSpecies,
		"NextSpecies": l.NextSpecies,
		"PrevDoctor":  l.PrevDoctor,
		"NextDoctor":  l.NextDoctor,
	} {
		if v != NilOffset {
			t.Errorf("%s = %d, want NilOffset", name, v)
		}
	}
}
