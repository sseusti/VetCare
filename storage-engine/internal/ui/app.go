// Package ui is the Fyne desktop interface of the database.
//
// The UI never touches files itself: it only calls package db. The rows it
// keeps in memory are a copy for drawing the table, which the assignment
// allows; every operation still goes to the database files.
package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/sseusti/VetCare/storage-engine/internal/db"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// App is the main window and its state.
type App struct {
	app fyne.App
	win fyne.Window

	db        *db.DB
	rows      []record.Visit // what the table shows: all visits or search results
	selected  int            // selected table row, -1 if none
	filtered  bool           // rows are search results, not the whole table
	lastField db.Field       // the search shown when filtered
	lastValue string

	table      *widget.Table
	status     *widget.Label
	fieldSel   *widget.Select
	valueEntry *widget.SelectEntry
}

// New builds the main window. Call Window().ShowAndRun() to start.
func New(a fyne.App) *App {
	u := &App{app: a, win: a.NewWindow("VetCare"), selected: -1}
	u.win.SetMainMenu(u.mainMenu())
	u.win.SetContent(u.layout())
	u.win.Resize(fyne.NewSize(1100, 650))
	u.win.SetOnClosed(func() {
		if u.db != nil {
			u.db.Close()
		}
	})
	u.refreshTitle()
	u.setStatus("Создайте или откройте базу через меню «Файл»")
	return u
}

// Window returns the main window.
func (u *App) Window() fyne.Window { return u.win }

func (u *App) layout() fyne.CanvasObject {
	u.table = u.newTable()
	u.status = widget.NewLabel("")
	u.status.Wrapping = fyne.TextWrapWord

	u.fieldSel = widget.NewSelect(fieldNames(), func(string) { u.updateValueOptions() })
	u.fieldSel.SetSelectedIndex(0)
	u.valueEntry = widget.NewSelectEntry(nil)
	u.valueEntry.SetPlaceHolder("Значение")
	u.valueEntry.OnSubmitted = func(string) { u.onSearch() }
	u.updateValueOptions()

	search := container.NewBorder(nil, nil,
		container.NewHBox(widget.NewLabel("Поиск по"), u.fieldSel),
		container.NewHBox(
			widget.NewButton("Найти", u.onSearch),
			widget.NewButton("Удалить найденные", u.onDeleteFound),
			widget.NewButton("Показать все", u.onReset),
		),
		u.valueEntry,
	)
	records := container.NewHBox(
		widget.NewButton("Добавить", u.onAdd),
		widget.NewButton("Редактировать", u.onEdit),
		widget.NewButton("Удалить выбранную", u.onDeleteSelected),
	)
	top := container.NewVBox(search, records, widget.NewSeparator())
	return container.NewBorder(top, u.status, nil, nil, u.table)
}

func (u *App) mainMenu() *fyne.MainMenu {
	file := fyne.NewMenu("Файл",
		fyne.NewMenuItem("Создать базу…", u.onCreate),
		fyne.NewMenuItem("Открыть базу…", u.onOpen),
		fyne.NewMenuItem("Сохранить", u.onSave),
		fyne.NewMenuItem("Закрыть базу", u.onCloseDB),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Создать резервную копию", u.onBackup),
		fyne.NewMenuItem("Восстановить из копии…", u.onRestore),
		fyne.NewMenuItem("Экспорт в Excel…", u.onExport),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Очистить базу…", u.onClear),
		fyne.NewMenuItem("Удалить базу…", u.onDrop),
	)
	records := fyne.NewMenu("Записи",
		fyne.NewMenuItem("Добавить…", u.onAdd),
		fyne.NewMenuItem("Редактировать выбранную…", u.onEdit),
		fyne.NewMenuItem("Удалить выбранную", u.onDeleteSelected),
	)
	return fyne.NewMainMenu(file, records)
}

// setDB makes d the open database, closing the previous one, and shows all
// its visits.
func (u *App) setDB(d *db.DB) {
	if u.db != nil && u.db != d {
		u.db.Close()
	}
	u.db = d
	u.refreshTitle()
	u.showAll()
}

// showAll reloads every visit into the table: one Scan, O(n).
func (u *App) showAll() {
	u.rows = u.rows[:0]
	u.filtered = false
	if u.db != nil {
		start := time.Now()
		err := u.db.Scan(func(v record.Visit) bool {
			u.rows = append(u.rows, v)
			return true
		})
		if err != nil {
			u.showError(err)
		}
		u.setStatus(fmt.Sprintf("Загружено записей: %d за %s", len(u.rows), formatDuration(time.Since(start))))
	}
	u.redrawTable()
}

// reload refreshes the table after a change, keeping a search if there was one.
func (u *App) reload() {
	if u.filtered {
		u.search(u.lastField, u.lastValue)
		return
	}
	u.showAll()
}

func (u *App) redrawTable() {
	u.selected = -1
	u.table.UnselectAll()
	u.table.Refresh()
}

func (u *App) refreshTitle() {
	if u.db == nil {
		u.win.SetTitle("VetCare — база не открыта")
		return
	}
	u.win.SetTitle("VetCare — " + u.db.Dir())
}

func (u *App) setStatus(msg string) {
	if u.db != nil {
		msg = fmt.Sprintf("Записей в базе: %d · %s", u.db.Len(), msg)
	}
	u.status.SetText(msg)
}

// needDB reports whether a database is open, telling the user if not.
func (u *App) needDB() bool {
	if u.db == nil {
		u.showError(errNoDB)
		return false
	}
	return true
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%d мкс", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%.1f мс", float64(d.Microseconds())/1000)
	default:
		return fmt.Sprintf("%.2f с", d.Seconds())
	}
}
