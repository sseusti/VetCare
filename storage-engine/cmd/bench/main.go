// Command bench measures the database operations on databases of growing
// size and writes the results to a CSV file for the report.
//
// For every size N it builds a fresh database of N random visits plus a
// fixed probe group of 10 visits (one doctor, one pet name), then times:
//
//	insert            new visits                         O(1)
//	get_by_id         random existing IDs                O(1)
//	find_doctor_k10   probe doctor, always 10 matches    O(1 + k), k = 10
//	update            random IDs, doctor changes         O(1)
//	delete_by_id      random existing IDs                O(1)
//	find_species      a species, k ≈ N/10 matches        O(1 + k)
//	find_pet_name     probe pet name, full scan          O(n)
//	delete_doctor_k10 delete the probe group by doctor   O(k), k = 10
//
// Usage:
//
//	go run ./cmd/bench -sizes 1000,10000,100000,1000000 -out ../docs/lab2/bench.csv
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sseusti/VetCare/storage-engine/internal/db"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

const (
	probeDoctor = "Контрольный врач"
	probePet    = "Контрольная кличка"
	probeSize   = 10
)

var (
	species = []string{"Кошка", "Собака", "Попугай", "Хомяк", "Кролик",
		"Черепаха", "Морская свинка", "Шиншилла", "Хорёк", "Канарейка"}
	pets = []string{"Барсик", "Мурка", "Шарик", "Рекс", "Кеша", "Пушок", "Васька",
		"Бобик", "Симба", "Тайсон", "Луна", "Белка", "Граф", "Джек", "Соня"}
	doctors = makeDoctors(50)
)

func makeDoctors(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("Врач %02d", i+1)
	}
	return out
}

// result is one row of the CSV.
type result struct {
	records int
	op      string
	ops     int
	perOp   time.Duration
}

func main() {
	sizesFlag := flag.String("sizes", "1000,10000,100000,1000000", "comma-separated database sizes")
	ops := flag.Int("ops", 1000, "operations per O(1) measurement")
	scans := flag.Int("scans", 3, "repetitions of O(n) and O(k) searches")
	out := flag.String("out", "bench.csv", "CSV file to write")
	flag.Parse()

	sizes, err := parseSizes(*sizesFlag)
	if err != nil {
		log.Fatal(err)
	}
	tmp, err := os.MkdirTemp("", "vetdb-bench-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	var all []result
	for _, n := range sizes {
		res, err := runSize(filepath.Join(tmp, strconv.Itoa(n)), n, *ops, *scans)
		if err != nil {
			log.Fatalf("size %d: %v", n, err)
		}
		all = append(all, res...)
		printTable(res)
	}
	if err := writeCSV(*out, all); err != nil {
		log.Fatal(err)
	}
	fmt.Println("results written to", *out)
}

func runSize(dir string, n, ops, scans int) ([]result, error) {
	rng := rand.New(rand.NewPCG(uint64(n), 42))
	d, err := db.Create(dir)
	if err != nil {
		return nil, err
	}
	defer d.Drop()

	var res []result
	add := func(op string, count int, total time.Duration) {
		res = append(res, result{records: n, op: op, ops: count, perOp: total / time.Duration(count)})
	}

	// Bulk load: N random visits plus the probe group.
	fmt.Printf("\n== %d records: loading...\n", n)
	start := time.Now()
	for id := 1; id <= n; id++ {
		if err := d.Insert(randomVisit(rng, int32(id))); err != nil {
			return nil, err
		}
	}
	for i := 1; i <= probeSize; i++ {
		v := randomVisit(rng, int32(n+i))
		v.Doctor, v.PetName = probeDoctor, probePet
		if err := d.Insert(v); err != nil {
			return nil, err
		}
	}
	add("bulk_load", n+probeSize, time.Since(start))
	size, err := dirSize(dir)
	if err != nil {
		return nil, err
	}
	fmt.Printf("loaded in %s, files take %.1f MB\n", time.Since(start).Round(time.Millisecond), float64(size)/1e6)

	ops = min(ops, n/2) // update and delete need ops distinct existing IDs
	nextID := int32(n + probeSize + 1)

	start = time.Now()
	for i := 0; i < ops; i++ {
		if err := d.Insert(randomVisit(rng, nextID)); err != nil {
			return nil, err
		}
		nextID++
	}
	add("insert", ops, time.Since(start))

	start = time.Now()
	for i := 0; i < ops; i++ {
		if _, err := d.Get(int32(rng.IntN(n) + 1)); err != nil {
			return nil, err
		}
	}
	add("get_by_id", ops, time.Since(start))

	start = time.Now()
	for i := 0; i < ops; i++ {
		found, err := d.FindBy(db.FieldDoctor, probeDoctor)
		if err != nil {
			return nil, err
		}
		if len(found) != probeSize {
			return nil, fmt.Errorf("probe doctor: found %d, want %d", len(found), probeSize)
		}
	}
	add("find_doctor_k10", ops, time.Since(start))

	// Random distinct IDs for update and delete.
	perm := rng.Perm(n)
	start = time.Now()
	for i := 0; i < ops; i++ {
		id := int32(perm[i] + 1)
		v := randomVisit(rng, id)
		if err := d.Update(id, v); err != nil {
			return nil, err
		}
	}
	add("update", ops, time.Since(start))

	start = time.Now()
	for i := 0; i < ops; i++ {
		if err := d.DeleteByID(int32(perm[i] + 1)); err != nil {
			return nil, err
		}
	}
	add("delete_by_id", ops, time.Since(start))

	start = time.Now()
	for i := 0; i < scans; i++ {
		if _, err := d.FindBy(db.FieldSpecies, species[i%len(species)]); err != nil {
			return nil, err
		}
	}
	add("find_species", scans, time.Since(start))

	start = time.Now()
	for i := 0; i < scans; i++ {
		found, err := d.FindBy(db.FieldPetName, probePet)
		if err != nil {
			return nil, err
		}
		if len(found) != probeSize {
			return nil, fmt.Errorf("probe pet: found %d, want %d", len(found), probeSize)
		}
	}
	add("find_pet_name", scans, time.Since(start))

	start = time.Now()
	deleted, err := d.DeleteBy(db.FieldDoctor, probeDoctor)
	if err != nil {
		return nil, err
	}
	if deleted != probeSize {
		return nil, fmt.Errorf("delete probe doctor: deleted %d, want %d", deleted, probeSize)
	}
	add("delete_doctor_k10", 1, time.Since(start))

	return res, nil
}

func randomVisit(rng *rand.Rand, id int32) record.Visit {
	return record.Visit{
		ID:      id,
		PetName: pets[rng.IntN(len(pets))],
		Species: species[rng.IntN(len(species))],
		Doctor:  doctors[rng.IntN(len(doctors))],
		At:      time.Unix(1_767_225_600+int64(rng.IntN(365*24))*3600, 0),
		Status:  record.AllStatuses()[rng.IntN(len(record.AllStatuses()))],
		Price:   float64(500 + rng.IntN(50)*100),
	}
}

func parseSizes(s string) ([]int, error) {
	var out []int
	for _, p := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("bad size %q", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func dirSize(dir string) (int64, error) {
	var total int64
	for _, name := range db.Files {
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			return 0, err
		}
		total += st.Size()
	}
	return total, nil
}

func printTable(res []result) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "operation\tops\ttime per op\t")
	for _, r := range res {
		fmt.Fprintf(w, "%s\t%d\t%s\t\n", r.op, r.ops, r.perOp.Round(100*time.Nanosecond))
	}
	w.Flush()
}

func writeCSV(path string, res []result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	w.Write([]string{"records", "operation", "ops", "ns_per_op"})
	for _, r := range res {
		w.Write([]string{strconv.Itoa(r.records), r.op, strconv.Itoa(r.ops), strconv.FormatInt(r.perOp.Nanoseconds(), 10)})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
