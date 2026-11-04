package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sseusti/VetCare/storage-engine/internal/db"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

var now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)

func TestBuildCreatesOpenableDatabase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	n, err := build(dir, 300, 1, now)
	if err != nil || n != 300 {
		t.Fatalf("build: %d, %v", n, err)
	}

	d, err := db.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()
	if d.Len() != 300 {
		t.Fatalf("Len = %d, want 300", d.Len())
	}

	// Every visit is valid, past visits are not scheduled, future ones are.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	err = d.Scan(func(v record.Visit) bool {
		if err := record.Validate(v); err != nil {
			t.Errorf("visit %d: %v", v.ID, err)
		}
		future := !v.At.Before(today)
		if future != (v.Status == record.StatusScheduled) {
			t.Errorf("visit %d at %v has status %v", v.ID, v.At, v.Status)
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}

	// Indexed search finds something for a common doctor and species.
	for _, q := range []struct {
		f     db.Field
		value string
	}{{db.FieldDoctor, doctors[0]}, {db.FieldSpecies, "Кошка"}} {
		found, err := d.FindBy(q.f, q.value)
		if err != nil || len(found) == 0 {
			t.Errorf("FindBy(%v, %q) = %d visits, %v", q.f, q.value, len(found), err)
		}
	}
}

func TestBuildIsReproducible(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a")
	b := filepath.Join(t.TempDir(), "b")
	if _, err := build(a, 50, 7, now); err != nil {
		t.Fatal(err)
	}
	if _, err := build(b, 50, 7, now); err != nil {
		t.Fatal(err)
	}
	da, _ := db.Open(a)
	defer da.Close()
	dbb, _ := db.Open(b)
	defer dbb.Close()
	for id := int32(1); id <= 50; id++ {
		va, _ := da.Get(id)
		vb, _ := dbb.Get(id)
		if va != vb {
			t.Fatalf("visit %d differs: %+v vs %+v", id, va, vb)
		}
	}
}

func TestBuildRefusesExistingDatabase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	if _, err := build(dir, 10, 1, now); err != nil {
		t.Fatal(err)
	}
	if _, err := build(dir, 10, 1, now); !errors.Is(err, db.ErrExists) {
		t.Fatalf("second build: %v, want ErrExists", err)
	}
}

func TestBuildRejectsNonPositiveN(t *testing.T) {
	if _, err := build(filepath.Join(t.TempDir(), "x"), 0, 1, now); err == nil {
		t.Fatal("want an error for n = 0")
	}
}
