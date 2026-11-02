package ui

import (
	"errors"
	"strings"

	"fyne.io/fyne/v2/dialog"

	"github.com/sseusti/VetCare/storage-engine/internal/db"
	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

var errNoDB = errors.New("сначала создайте или откройте базу через меню «Файл»")

// humanError turns a database error into a message for the user. Unknown
// errors are shown as they are.
func humanError(err error) string {
	switch {
	case errors.Is(err, db.ErrDuplicateKey):
		return "Приём с таким ID уже есть: ID должен быть уникальным"
	case errors.Is(err, db.ErrNotFound):
		return "Приём с таким ID не найден"
	case errors.Is(err, db.ErrExists):
		return "В этой папке уже есть база данных"
	case errors.Is(err, db.ErrNotDB):
		return "В этой папке нет базы данных VetCare"
	case errors.Is(err, db.ErrCorrupt):
		return "Файлы базы повреждены или не согласованы между собой"
	case errors.Is(err, db.ErrBadValue):
		return "Некорректное значение для поиска: " + afterColon(err)
	case errors.Is(err, db.ErrClosed):
		return errNoDB.Error()
	case errors.Is(err, record.ErrInvalid):
		return "Некорректные данные: " + afterColon(err)
	}
	return err.Error()
}

// afterColon drops the "package: sentinel:" prefix of a wrapped error.
func afterColon(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		return msg[i+2:]
	}
	return msg
}

func (u *App) showError(err error) {
	dialog.ShowError(errors.New(humanError(err)), u.win)
}
