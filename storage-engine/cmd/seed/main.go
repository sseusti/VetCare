// Command seed creates a demo database filled with realistic visits, so the
// GUI can be tried right away: open the folder with File → Open database.
//
// Usage:
//
//	go run ./cmd/seed -out ~/vetcare-demo           # 300 visits
//	go run ./cmd/seed -out ~/vetcare-demo -n 5000   # more visits
//
// The same -seed always gives the same database. IDs go from 1 to n.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"time"

	"github.com/sseusti/VetCare/storage-engine/internal/db"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// weighted is a value with its relative frequency.
type weighted struct {
	value  string
	weight int
}

var (
	species = []weighted{
		{"Кошка", 40}, {"Собака", 35}, {"Попугай", 6}, {"Кролик", 5}, {"Хомяк", 4},
		{"Морская свинка", 4}, {"Хорёк", 3}, {"Черепаха", 3},
	}
	petNames = map[string][]string{
		"Кошка":          {"Барсик", "Мурка", "Васька", "Симба", "Луна", "Соня", "Пушок", "Маркиз", "Ириска", "Тиша"},
		"Собака":         {"Рекс", "Шарик", "Тайсон", "Джек", "Белка", "Граф", "Лаки", "Найда", "Бим", "Марта"},
		"Попугай":        {"Кеша", "Гоша", "Рико", "Чика"},
		"Кролик":         {"Пушистик", "Ушастик", "Снежок", "Крош"},
		"Хомяк":          {"Хома", "Персик", "Бублик", "Орешек"},
		"Морская свинка": {"Пеппа", "Булка", "Фунтик"},
		"Хорёк":          {"Фокс", "Шустрик", "Ласка"},
		"Черепаха":       {"Тортилла", "Наполеон", "Шелли"},
	}
	doctors = []string{
		"Иванова А. С.", "Петров Д. В.", "Смирнова Е. К.", "Кузнецов И. А.",
		"Волкова М. П.", "Соколов А. Н.", "Морозова О. В.", "Новиков Г. Е.",
	}
	// Price list of typical services, rubles.
	prices = []float64{900, 1200, 1500, 1800, 2500, 3200, 4500, 7000, 12000}
)

func main() {
	out := flag.String("out", "", "folder for the new database (required, must not contain a database)")
	n := flag.Int("n", 300, "number of visits")
	seed := flag.Uint64("seed", 1, "random seed: the same seed gives the same data")
	flag.Parse()

	if *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	count, err := build(*out, *n, *seed, time.Now())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Готово: %d приёмов в %s\nОткройте папку в приложении: Файл → Открыть базу…\n", count, *out)
}

// build creates a database in dir with n visits around now: the past two
// months (visited, cancelled or no-show) and the next two weeks (scheduled).
func build(dir string, n int, seed uint64, now time.Time) (int, error) {
	if n <= 0 {
		return 0, errors.New("n must be positive")
	}
	d, err := db.Create(dir)
	if err != nil {
		return 0, err
	}
	defer d.Close()

	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	for i := 1; i <= n; i++ {
		if err := d.Insert(visit(rng, int32(i), today)); err != nil {
			return i - 1, fmt.Errorf("visit %d: %w", i, err)
		}
	}
	return n, d.Sync()
}

func visit(rng *rand.Rand, id int32, today time.Time) record.Visit {
	sp := pick(rng, species)
	names := petNames[sp]
	day := today.AddDate(0, 0, rng.IntN(75)-60) // 60 days back .. 14 days ahead
	at := day.Add(time.Duration(9*60+30*rng.IntN(22)) * time.Minute)

	status := record.StatusScheduled
	if day.Before(today) {
		switch r := rng.IntN(100); {
		case r < 85:
			status = record.StatusVisited
		case r < 92:
			status = record.StatusCancelled
		default:
			status = record.StatusNoShow
		}
	}

	return record.Visit{
		ID:      id,
		PetName: names[rng.IntN(len(names))],
		Species: sp,
		Doctor:  doctors[rng.IntN(len(doctors))],
		At:      at,
		Status:  status,
		Price:   prices[rng.IntN(len(prices))],
	}
}

func pick(rng *rand.Rand, items []weighted) string {
	total := 0
	for _, it := range items {
		total += it.weight
	}
	r := rng.IntN(total)
	for _, it := range items {
		if r < it.weight {
			return it.value
		}
		r -= it.weight
	}
	return items[len(items)-1].value
}
