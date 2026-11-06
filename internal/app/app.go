package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type App struct {
	err string
}

func Run() error {
	a := &App{}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title: "Folio", Width: 1040, Height: 720,
			MinWidth: 640, MinHeight: 400, Content: ui.View(a.view),
		})
	})
	return mygo.App.Run()
}

func (a *App) notify(err error) {
	if err != nil {
		a.err = err.Error()
	}
}
