// Command vetdb starts the VetCare file database GUI.
package main

import (
	"fyne.io/fyne/v2/app"

	"github.com/sseusti/VetCare/storage-engine/internal/ui"
)

func main() {
	a := app.NewWithID("com.sseusti.vetcare")
	ui.New(a).Window().ShowAndRun()
}
