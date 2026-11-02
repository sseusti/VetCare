package record

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestValidateOK(t *testing.T) {
	if err := Validate(sampleVisit()); err != nil {
		t.Fatalf("valid visit rejected: %v", err)
	}
	v := sampleVisit()
	v.PetName = strings.Repeat("Ж", PetNameSize/2) // exactly 48 bytes
	if err := Validate(v); err != nil {
		t.Fatalf("name at the byte limit rejected: %v", err)
	}
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Visit)
	}{
		{"zero id", func(v *Visit) { v.ID = 0 }},
		{"negative id", func(v *Visit) { v.ID = -5 }},
		{"empty pet name", func(v *Visit) { v.PetName = "" }},
		{"blank species", func(v *Visit) { v.Species = "   " }},
		{"empty doctor", func(v *Visit) { v.Doctor = "" }},
		{"pet name too long", func(v *Visit) { v.PetName = strings.Repeat("Ж", 25) }},
		{"species too long", func(v *Visit) { v.Species = strings.Repeat("a", SpeciesSize+1) }},
		{"doctor too long", func(v *Visit) { v.Doctor = strings.Repeat("Я", 33) }},
		{"invalid utf8", func(v *Visit) { v.PetName = "Бар\xffсик" }},
		{"zero byte", func(v *Visit) { v.Doctor = "Иванова\x00" }},
		{"zero time", func(v *Visit) { v.At = time.Time{} }},
		{"zero status", func(v *Visit) { v.Status = 0 }},
		{"unknown status", func(v *Visit) { v.Status = 99 }},
		{"negative price", func(v *Visit) { v.Price = -1 }},
		{"NaN price", func(v *Visit) { v.Price = math.NaN() }},
		{"infinite price", func(v *Visit) { v.Price = math.Inf(1) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := sampleVisit()
			tc.mutate(&v)
			if err := Validate(v); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}
