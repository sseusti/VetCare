// Command vetdb starts the VetCare file database GUI.
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func main() {
	a := app.NewWithID("com.sseusti.vetcare")
	w := a.NewWindow("VetCare")
	w.SetContent(container.NewCenter(widget.NewLabel("VetCare")))
	w.Resize(fyne.NewSize(900, 600))
	w.ShowAndRun()
}
