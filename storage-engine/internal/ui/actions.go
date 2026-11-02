package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/sseusti/VetCare/storage-engine/internal/db"
	"github.com/sseusti/VetCare/storage-engine/internal/export"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// ---- database file actions ------------------------------------------------

func (u *App) onCreate() {
	dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
		if err != nil || dir == nil {
			return
		}
		name := widget.NewEntry()
		name.SetText("vetcare-db")
		dialog.ShowForm("Новая база", "Создать", "Отмена",
			[]*widget.FormItem{widget.NewFormItem("Название папки", name)},
			func(ok bool) {
				if ok {
					u.createDB(filepath.Join(dir.Path(), strings.TrimSpace(name.Text)))
				}
			}, u.win)
	}, u.win)
}

func (u *App) createDB(dir string) {
	d, err := db.Create(dir)
	if err != nil {
		u.showError(err)
		return
	}
	u.setDB(d)
	u.setStatus("Создана новая база: " + dir)
}

func (u *App) onOpen() {
	dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
		if err != nil || dir == nil {
			return
		}
		u.openDB(dir.Path())
	}, u.win)
}

func (u *App) openDB(dir string) {
	d, err := db.Open(dir)
	if err != nil {
		u.showError(err)
		return
	}
	u.setDB(d)
}

func (u *App) onSave() {
	if !u.needDB() {
		return
	}
	if err := u.db.Sync(); err != nil {
		u.showError(err)
		return
	}
	u.setStatus("Сохранено на диск")
}

func (u *App) onCloseDB() {
	if u.db == nil {
		return
	}
	u.db.Close()
	u.db = nil
	u.rows = nil
	u.redrawTable()
	u.refreshTitle()
	u.setStatus("База закрыта")
}

func (u *App) onClear() {
	if !u.needDB() {
		return
	}
	dialog.ShowConfirm("Очистить базу",
		fmt.Sprintf("Удалить все записи (%d)? Файлы базы останутся.", u.db.Len()),
		func(ok bool) {
			if ok {
				u.clearDB()
			}
		}, u.win)
}

func (u *App) clearDB() {
	if err := u.db.Clear(); err != nil {
		u.showError(err)
		return
	}
	u.showAll()
	u.setStatus("База очищена")
}

func (u *App) onDrop() {
	if !u.needDB() {
		return
	}
	dialog.ShowConfirm("Удалить базу",
		"Удалить файлы базы "+u.db.Dir()+"? Это нельзя отменить.",
		func(ok bool) {
			if ok {
				u.dropDB()
			}
		}, u.win)
}

func (u *App) dropDB() {
	dir := u.db.Dir()
	if err := u.db.Drop(); err != nil {
		u.showError(err)
		return
	}
	u.db = nil
	u.rows = nil
	u.redrawTable()
	u.refreshTitle()
	u.setStatus("База удалена: " + dir)
}

// ---- backup and export ------------------------------------------------------

func (u *App) onBackup() {
	if !u.needDB() {
		return
	}
	dest, err := u.backup(time.Now())
	if err != nil {
		u.showError(err)
		return
	}
	u.setStatus("Резервная копия: " + dest)
	dialog.ShowInformation("Резервная копия", "Копия сохранена в папку\n"+dest, u.win)
}

// backup writes a copy next to the database folder, named after it and the
// current time: vetcare-db-backup-20261102-153000.
func (u *App) backup(now time.Time) (string, error) {
	dir := filepath.Clean(u.db.Dir())
	dest := filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-backup-"+now.Format("20060102-150405"))
	return dest, u.db.Backup(dest)
}

func (u *App) onRestore() {
	if !u.needDB() {
		return
	}
	dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
		if err != nil || dir == nil {
			return
		}
		dialog.ShowConfirm("Восстановление",
			"Заменить текущую базу копией из\n"+dir.Path()+"?\nТекущие данные будут потеряны.",
			func(ok bool) {
				if ok {
					u.restore(dir.Path())
				}
			}, u.win)
	}, u.win)
}

func (u *App) restore(src string) {
	if err := u.db.Restore(src); err != nil {
		u.showError(err)
		return
	}
	u.showAll()
	u.setStatus("База восстановлена из " + src)
}

func (u *App) onExport() {
	if !u.needDB() {
		return
	}
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		path := w.URI().Path()
		w.Close() // excelize writes the file itself, by path
		u.exportTo(path)
	}, u.win)
	d.SetFileName("visits.xlsx")
	d.SetFilter(storage.NewExtensionFileFilter([]string{".xlsx"}))
	d.Show()
}

func (u *App) exportTo(path string) {
	if !strings.HasSuffix(strings.ToLower(path), ".xlsx") {
		os.Remove(path) // the empty file the save dialog created
		path += ".xlsx"
	}
	start := time.Now()
	n, err := export.ToXLSX(u.db, path)
	if err != nil {
		u.showError(err)
		return
	}
	u.setStatus(fmt.Sprintf("Экспортировано строк: %d за %s → %s", n, formatDuration(time.Since(start)), path))
}

// ---- records ------------------------------------------------------------------

func (u *App) onAdd() {
	if !u.needDB() {
		return
	}
	initial := visitInput{
		At:     time.Now().Truncate(time.Minute).Format(DateLayout),
		Status: record.StatusScheduled.String(),
		Price:  "0",
	}
	u.showVisitForm("Новый приём", initial, u.insert)
}

func (u *App) insert(v record.Visit) error {
	start := time.Now()
	if err := u.db.Insert(v); err != nil {
		return err
	}
	took := time.Since(start)
	u.reload()
	u.setStatus(fmt.Sprintf("Добавлен приём %d за %s", v.ID, formatDuration(took)))
	return nil
}

func (u *App) onEdit() {
	if !u.needDB() {
		return
	}
	v, ok := u.selectedVisit()
	if !ok {
		u.showError(fmt.Errorf("выберите строку в таблице"))
		return
	}
	u.showVisitForm(fmt.Sprintf("Приём %d", v.ID), inputOf(v), func(nv record.Visit) error {
		return u.update(v.ID, nv)
	})
}

func (u *App) update(id int32, v record.Visit) error {
	start := time.Now()
	if err := u.db.Update(id, v); err != nil {
		return err
	}
	took := time.Since(start)
	u.reload()
	u.setStatus(fmt.Sprintf("Приём %d изменён за %s", id, formatDuration(took)))
	return nil
}

func (u *App) onDeleteSelected() {
	if !u.needDB() {
		return
	}
	v, ok := u.selectedVisit()
	if !ok {
		u.showError(fmt.Errorf("выберите строку в таблице"))
		return
	}
	dialog.ShowConfirm("Удаление", fmt.Sprintf("Удалить приём %d (%s, %s)?", v.ID, v.PetName, v.Species),
		func(ok bool) {
			if ok {
				u.deleteByID(v.ID)
			}
		}, u.win)
}

func (u *App) deleteByID(id int32) {
	start := time.Now()
	if err := u.db.DeleteByID(id); err != nil {
		u.showError(err)
		return
	}
	took := time.Since(start)
	u.reload()
	u.setStatus(fmt.Sprintf("Приём %d удалён за %s", id, formatDuration(took)))
}

// ---- search -------------------------------------------------------------------

func fieldNames() []string {
	var out []string
	for _, f := range db.Fields() {
		out = append(out, f.String())
	}
	return out
}

func (u *App) currentField() db.Field {
	return db.Fields()[max(u.fieldSel.SelectedIndex(), 0)]
}

// updateValueOptions offers suggestions for the chosen field.
func (u *App) updateValueOptions() {
	if u.valueEntry == nil {
		return
	}
	switch u.currentField() {
	case db.FieldStatus:
		u.valueEntry.SetOptions(statusNames())
	case db.FieldSpecies:
		u.valueEntry.SetOptions(commonSpecies)
	default:
		u.valueEntry.SetOptions(nil)
	}
}

func (u *App) onSearch() {
	if !u.needDB() {
		return
	}
	u.search(u.currentField(), u.valueEntry.Text)
}

func (u *App) search(f db.Field, value string) {
	start := time.Now()
	found, err := u.db.FindBy(f, value)
	took := time.Since(start)
	if err != nil {
		u.showError(err)
		return
	}
	u.rows = found
	u.filtered = true
	u.lastField, u.lastValue = f, value
	u.redrawTable()
	u.setStatus(fmt.Sprintf("%s = «%s»: найдено %d за %s (%s)",
		f, strings.TrimSpace(value), len(found), formatDuration(took), method(f)))
}

func (u *App) onDeleteFound() {
	if !u.needDB() {
		return
	}
	f, value := u.currentField(), u.valueEntry.Text
	found, err := u.db.FindBy(f, value)
	if err != nil {
		u.showError(err)
		return
	}
	if len(found) == 0 {
		u.setStatus(fmt.Sprintf("%s = «%s»: ничего не найдено", f, strings.TrimSpace(value)))
		return
	}
	dialog.ShowConfirm("Удаление",
		fmt.Sprintf("Удалить все приёмы, где %s = «%s» (%d шт.)?", f, strings.TrimSpace(value), len(found)),
		func(ok bool) {
			if ok {
				u.deleteBy(f, value)
			}
		}, u.win)
}

func (u *App) deleteBy(f db.Field, value string) {
	start := time.Now()
	n, err := u.db.DeleteBy(f, value)
	took := time.Since(start)
	if err != nil {
		u.showError(err)
	}
	u.showAll()
	u.setStatus(fmt.Sprintf("Удалено приёмов: %d за %s (%s)", n, formatDuration(took), method(f)))
}

func (u *App) onReset() {
	if !u.needDB() {
		return
	}
	u.valueEntry.SetText("")
	u.showAll()
}

// method says how a field is searched, for the status line.
func method(f db.Field) string {
	switch {
	case f == db.FieldID:
		return "хеш-индекс по ключу"
	case f.Indexed():
		return "вторичный индекс"
	default:
		return "полный проход по файлу"
	}
}
