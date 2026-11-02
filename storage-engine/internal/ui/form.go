package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/sseusti/VetCare/storage-engine/internal/record"
)

// commonSpecies are suggestions; any other species can be typed.
var commonSpecies = []string{"Кошка", "Собака", "Попугай", "Хомяк", "Кролик", "Черепаха"}

// visitInput is the text of the visit form, before parsing.
type visitInput struct {
	ID, PetName, Species, Doctor, At, Status, Price string
}

func inputOf(v record.Visit) visitInput {
	return visitInput{
		ID:      strconv.Itoa(int(v.ID)),
		PetName: v.PetName,
		Species: v.Species,
		Doctor:  v.Doctor,
		At:      v.At.Local().Format(DateLayout),
		Status:  v.Status.String(),
		Price:   strconv.FormatFloat(v.Price, 'f', -1, 64),
	}
}

// parseVisit converts the form text into a visit and validates it, with
// error messages a user can act on.
func parseVisit(in visitInput) (record.Visit, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(in.ID), 10, 32)
	if err != nil {
		return record.Visit{}, errors.New("ID должен быть целым числом")
	}
	at, err := time.ParseInLocation(DateLayout, strings.TrimSpace(in.At), time.Local)
	if err != nil {
		return record.Visit{}, fmt.Errorf("дата должна быть в формате %s, например %s",
			"дд.мм.гггг чч:мм", time.Now().Format(DateLayout))
	}
	status, err := record.ParseStatus(in.Status)
	if err != nil {
		return record.Visit{}, errors.New("выберите статус из списка")
	}
	priceText := strings.ReplaceAll(strings.TrimSpace(in.Price), ",", ".")
	price, err := strconv.ParseFloat(priceText, 64)
	if err != nil {
		return record.Visit{}, errors.New("стоимость должна быть числом, например 1500 или 1500,50")
	}
	v := record.Visit{
		ID:      int32(id),
		PetName: strings.TrimSpace(in.PetName),
		Species: strings.TrimSpace(in.Species),
		Doctor:  strings.TrimSpace(in.Doctor),
		At:      at,
		Status:  status,
		Price:   price,
	}
	if err := record.Validate(v); err != nil {
		return record.Visit{}, err
	}
	return v, nil
}

// showVisitForm opens the add/edit form. On a parse or save error it shows
// the error and reopens the form with the text the user typed.
func (u *App) showVisitForm(title string, initial visitInput, save func(record.Visit) error) {
	id := widget.NewEntry()
	id.SetText(initial.ID)
	pet := widget.NewEntry()
	pet.SetText(initial.PetName)
	species := widget.NewSelectEntry(commonSpecies)
	species.SetText(initial.Species)
	doctor := widget.NewEntry()
	doctor.SetText(initial.Doctor)
	at := widget.NewEntry()
	at.SetText(initial.At)
	at.SetPlaceHolder("дд.мм.гггг чч:мм")
	status := widget.NewSelect(statusNames(), nil)
	status.SetSelected(initial.Status)
	price := widget.NewEntry()
	price.SetText(initial.Price)

	items := []*widget.FormItem{
		widget.NewFormItem("ID", id),
		widget.NewFormItem("Кличка", pet),
		widget.NewFormItem("Вид", species),
		widget.NewFormItem("Врач", doctor),
		widget.NewFormItem("Дата и время", at),
		widget.NewFormItem("Статус", status),
		widget.NewFormItem("Стоимость, ₽", price),
	}

	var open func()
	open = func() {
		d := dialog.NewForm(title, "Сохранить", "Отмена", items, func(ok bool) {
			if !ok {
				return
			}
			v, err := parseVisit(visitInput{
				ID: id.Text, PetName: pet.Text, Species: species.Text, Doctor: doctor.Text,
				At: at.Text, Status: status.Selected, Price: price.Text,
			})
			if err == nil {
				err = save(v)
			}
			if err != nil {
				e := dialog.NewError(errors.New(humanError(err)), u.win)
				e.SetOnClosed(open)
				e.Show()
			}
		}, u.win)
		d.Resize(d.MinSize().AddWidthHeight(200, 0))
		d.Show()
	}
	open()
}

func statusNames() []string {
	var out []string
	for _, s := range record.AllStatuses() {
		out = append(out, s.String())
	}
	return out
}
