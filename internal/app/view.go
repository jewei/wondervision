package app

import "github.com/egoist/mygo/ui"

func (a *App) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Padding(10, 16).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "Folio").Bold()
		})
		if a.err != "" {
			ui.Row(c).Padding(8, 16).Gap(12).Children(func() {
				ui.Text(c, a.err).Grow(1)
				ui.Button(c, "Dismiss").OnClick(func() { a.err = "" })
			})
		}
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(220).Padding(16).Background(t.Surface).Children(func() {
				ui.Text(c, "EXPLORER").FontSize(11).TextColor(t.TextMuted)
				ui.Text(c, "No folder open").TextColor(t.TextMuted)
			})
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "Open a file or folder to begin.").TextColor(t.TextMuted)
			})
		})
		ui.Row(c).Padding(6, 16).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "Ready").FontSize(12).TextColor(t.TextMuted)
		})
	})
}
