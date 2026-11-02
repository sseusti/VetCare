package record

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// ErrInvalid wraps every validation error, so callers can check it with
// errors.Is and still show the detailed message to the user.
var ErrInvalid = errors.New("invalid visit")

// Validate checks that v can be stored without losing data.
func Validate(v Visit) error {
	if v.ID <= 0 {
		return fmt.Errorf("%w: id must be positive, got %d", ErrInvalid, v.ID)
	}
	if err := validateString("pet name", v.PetName, PetNameSize); err != nil {
		return err
	}
	if err := validateString("species", v.Species, SpeciesSize); err != nil {
		return err
	}
	if err := validateString("doctor", v.Doctor, DoctorSize); err != nil {
		return err
	}
	if v.At.IsZero() {
		return fmt.Errorf("%w: visit time is not set", ErrInvalid)
	}
	if !v.Status.Valid() {
		return fmt.Errorf("%w: unknown status %d", ErrInvalid, uint8(v.Status))
	}
	if math.IsNaN(v.Price) || math.IsInf(v.Price, 0) || v.Price < 0 {
		return fmt.Errorf("%w: price must be a non-negative number, got %v", ErrInvalid, v.Price)
	}
	return nil
}

func validateString(field, s string, maxBytes int) error {
	switch {
	case strings.TrimSpace(s) == "":
		return fmt.Errorf("%w: %s is empty", ErrInvalid, field)
	case !utf8.ValidString(s):
		return fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalid, field)
	case strings.ContainsRune(s, 0):
		return fmt.Errorf("%w: %s contains a zero byte", ErrInvalid, field)
	case len(s) > maxBytes:
		return fmt.Errorf("%w: %s is %d bytes, max %d (a Cyrillic letter takes 2 bytes)",
			ErrInvalid, field, len(s), maxBytes)
	}
	return nil
}
