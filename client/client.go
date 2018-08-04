package main

import (
	"github.com/gofunky/goplayspace/client/component/app"
	"github.com/gofunky/goplayspace/client/js/localstorage"
	"github.com/gopherjs/vecty"
)

func main() {
	vecty.SetTitle(app.PageTitle)

	a := &app.Application{
		Theme:            localstorage.Get("theme", "space"),
		TabWidth:         localstorage.GetInt("tab-width", 4),
		FontWeight:       localstorage.Get("font-weight", "normal"),
		UseWebfont:       localstorage.GetBool("use-webfont", true),
		HighlightingMode: localstorage.GetBool("highlighting", true),
		ShowSidebar:      localstorage.GetBool("show-sidebar", false),
	}

	vecty.RenderBody(a)
}
