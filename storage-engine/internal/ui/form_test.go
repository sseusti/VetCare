package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sseusti/VetCare/storage-engine/internal/db"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

func goodInput() visitInput {
	return visitInput{
		ID: " 12 ", PetName: "Барсик", Species: "Кошка", Doctor: "Иванова А. П.",
		At: "02.11.2026 14:30", Status: "Состоялся", Price: "1500,50",
	}
}

func TestParseVisit(t *testing.T) {
	v, err := parseVisit(goodInput())
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != 12 || v.Price != 1500.5 || v.Status != record.StatusVisited {
		t.Fatalf("parsed %+v", v)
	}
	if want := time.Date(2026, 11, 2, 14, 30, 0, 0, time.Local); !v.At.Equal(want) {
		t.Fatalf("At = %v, want %v", v.At, want)
	}
}

func TestParseVisitErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*visitInput)
		want   string
	}{
		{"id not a number", func(in *visitInput) { in.ID = "abc" }, "ID"},
		{"bad date", func(in *visitInput) { in.At = "2026-11-02" }, "дата"},
		{"no status", func(in *visitInput) { in.Status = "" }, "статус"},
		{"bad price", func(in *visitInput) { in.Price = "дорого" }, "стоимость"},
		{"empty pet name", func(in *visitInput) { in.PetName = " " }, "pet name"},
		{"negative price", func(in *visitInput) { in.Price = "-1" }, "price"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := goodInput()
			tc.mutate(&in)
			_, err := parseVisit(in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestInputOfRoundTrip(t *testing.T) {
	v, _ := parseVisit(goodInput())
	back, err := parseVisit(inputOf(v))
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != v.ID || back.Price != v.Price || !back.At.Equal(v.At) || back.Species != v.Species {
		t.Fatalf("round trip %+v != %+v", back, v)
	}
}

func TestHumanError(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{db.ErrDuplicateKey, "уже есть"},
		{db.ErrNotFound, "не найден"},
		{db.ErrExists, "уже есть база"},
		{db.ErrNotDB, "нет базы"},
		{db.ErrCorrupt, "повреждены"},
		{db.ErrClosed, "откройте базу"},
		{errors.Join(db.ErrBadValue, errors.New("x")), "Некорректное значение"},
		{record.ErrInvalid, "Некорректные данные"},
		{errors.New("что-то иное"), "что-то иное"},
	}
	for _, tc := range tests {
		if got := humanError(tc.err); !strings.Contains(got, tc.want) {
			t.Errorf("humanError(%v) = %q, want it to contain %q", tc.err, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{15 * time.Microsecond, "15 мкс"},
		{2500 * time.Microsecond, "2.5 мс"},
		{1500 * time.Millisecond, "1.50 с"},
	}
	for _, tc := range tests {
		if got := formatDuration(tc.d); got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestMethod(t *testing.T) {
	if method(db.FieldID) != "хеш-индекс по ключу" || method(db.FieldDoctor) != "вторичный индекс" ||
		method(db.FieldStatus) != "полный проход по файлу" {
		t.Error("method names are wrong")
	}
}
