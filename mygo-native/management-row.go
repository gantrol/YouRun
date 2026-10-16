package main

import "github.com/egoist/mygo/ui"

type managementActions struct{ edit, visibility, remove bool }

func (a *app) managementRow(c *ui.Context, name string, visible *bool) managementActions {
	var actions managementActions
	ui.Row(c).FillWidth().MinHeight(48).Gap(12).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(a.theme.Border).PaddingY(6).Children(func() {
		nameButton := ui.ButtonBase(c.Key("name")).Grow(1).Shrink(1).Basis(0).MinWidth(0).MinHeight(32).Padding(4, 0).Justify(ui.Start).Label(name).Tooltip(name)
		if nameButton.Hovered() {
			nameButton.Background(a.theme.SurfaceHover)
		}
		nameButton.Children(func() { ui.Text(c, name).MaxLines(1).Shrink(1).MinWidth(0) })
		actions.edit = nameButton.Clicked()
		actions.visibility = a.checkbox(c.Key("visible"), visible, a.m("showOnHome")).Width(136).Shrink(0).Changed()
		actions.remove = a.button(c.Key("remove"), a.m("remove")).Width(76).Shrink(0).TextColor(a.theme.Danger).Clicked()
	})
	return actions
}
