// Package export writes the database to files of other formats.
package export

import (
	"errors"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// SheetName is the name of the sheet with visits.
const SheetName = "Приёмы"

// Header is the first row of the sheet.
var Header = []any{"ID", "Кличка", "Вид", "Врач", "Дата и время", "Статус", "Стоимость, ₽"}

var columnWidths = []float64{8, 20, 16, 28, 18, 16, 14}

// Scanner is anything that can walk over every visit; *db.DB satisfies it.
type Scanner interface {
	Scan(fn func(v record.Visit) bool) error
}

// ToXLSX writes every visit from src into an Excel file at path and returns
// how many rows were written. Rows go out one by one through a stream
// writer, so memory use does not grow with the size of the database. O(n).
func ToXLSX(src Scanner, path string) (rows int, err error) {
	f := excelize.NewFile()
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if err := f.SetSheetName("Sheet1", SheetName); err != nil {
		return 0, err
	}
	sw, err := f.NewStreamWriter(SheetName)
	if err != nil {
		return 0, err
	}
	if err := setLayout(f, sw); err != nil {
		return 0, err
	}

	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return 0, err
	}
	dateStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: ptr("dd.mm.yyyy hh:mm")})
	if err != nil {
		return 0, err
	}
	moneyStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: ptr("#,##0.00")})
	if err != nil {
		return 0, err
	}

	header := make([]any, len(Header))
	for i, h := range Header {
		header[i] = excelize.Cell{StyleID: bold, Value: h}
	}
	if err := sw.SetRow("A1", header); err != nil {
		return 0, err
	}

	var writeErr error
	scanErr := src.Scan(func(v record.Visit) bool {
		cell, err := excelize.CoordinatesToCellName(1, rows+2)
		if err != nil {
			writeErr = err
			return false
		}
		writeErr = sw.SetRow(cell, []any{
			v.ID,
			v.PetName,
			v.Species,
			v.Doctor,
			excelize.Cell{StyleID: dateStyle, Value: localTime(v.At)},
			v.Status.String(),
			excelize.Cell{StyleID: moneyStyle, Value: v.Price},
		})
		if writeErr != nil {
			return false
		}
		rows++
		return true
	})
	if err := errors.Join(scanErr, writeErr); err != nil {
		return rows, fmt.Errorf("export: %w", err)
	}

	if err := sw.Flush(); err != nil {
		return rows, err
	}
	if err := f.SaveAs(path); err != nil {
		return rows, err
	}
	return rows, nil
}

// setLayout sets column widths and freezes the header row. A stream writer
// needs both before the first row.
func setLayout(f *excelize.File, sw *excelize.StreamWriter) error {
	for i, w := range columnWidths {
		if err := sw.SetColWidth(i+1, i+1, w); err != nil {
			return err
		}
	}
	return sw.SetPanes(&excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})
}

// localTime drops the time zone: Excel has no zones and would otherwise
// show UTC.
func localTime(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
}

func ptr[T any](v T) *T { return &v }
