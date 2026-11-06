package app

import "github.com/egoist/mygo/ui"

// fileMenu builds the File menu from the command table.
func (a *App) fileMenu(c *ui.Context) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		for i, section := range fileMenuSections {
			if i > 0 {
				m.Separator()
			}
			for _, id := range section {
				a.menuItem(c, m, id)
			}
		}
	}
}
