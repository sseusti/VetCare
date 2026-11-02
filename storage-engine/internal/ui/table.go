package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// DateLayout is how dates are shown and typed.
const DateLayout = "02.01.2006 15:04"

var columns = []struct {
	title string
	width float32
}{
	{"ID", 70},
	{"Кличка", 150},
	{"Вид", 120},
	{"Врач", 200},
	{"Дата и время", 140},
	{"Статус", 120},
	{"Стоимость, ₽", 110},
}

// rowCells turns a visit into the text of its table cells.
func rowCells(v record.Visit) []string {
	return []string{
		fmt.Sprint(v.ID),
		v.PetName,
		v.Species,
		v.Doctor,
		v.At.Local().Format(DateLayout),
		v.Status.String(),
		fmt.Sprintf("%.2f", v.Price),
	}
}

func (u *App) newTable() *widget.Table {
	t := widget.NewTableWithHeaders(
		func() (int, int) { return len(u.rows), len(columns) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(rowCells(u.rows[id.Row])[id.Col])
		},
	)
	t.ShowHeaderColumn = false
	t.CreateHeader = func() fyne.CanvasObject {
		l := widget.NewLabel("")
		l.TextStyle = fyne.TextStyle{Bold: true}
		return l
	}
	t.UpdateHeader = func(id widget.TableCellID, o fyne.CanvasObject) {
		if id.Row < 0 && id.Col >= 0 {
			o.(*widget.Label).SetText(columns[id.Col].title)
		}
	}
	for i, c := range columns {
		t.SetColumnWidth(i, c.width)
	}
	t.OnSelected = func(id widget.TableCellID) { u.selected = id.Row }
	t.OnUnselected = func(widget.TableCellID) { u.selected = -1 }
	return t
}

// selectedVisit returns the visit in the selected row.
func (u *App) selectedVisit() (record.Visit, bool) {
	if u.selected < 0 || u.selected >= len(u.rows) {
		return record.Visit{}, false
	}
	return u.rows[u.selected], true
}
