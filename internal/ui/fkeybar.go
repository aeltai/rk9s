// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package ui

import (
	"fmt"

	"github.com/derailed/k9s/internal/config"
	"github.com/derailed/k9s/internal/rk9s"
	"github.com/derailed/tview"
)

// FKeyBar renders a persistent F-key navigation legend at the bottom of the TUI.
type FKeyBar struct {
	*tview.TextView
	styles *config.Styles
}

// NewFKeyBar returns a new F-key bar.
func NewFKeyBar(styles *config.Styles) *FKeyBar {
	f := &FKeyBar{
		TextView: tview.NewTextView(),
		styles:   styles,
	}
	f.SetBackgroundColor(styles.BgColor())
	f.SetDynamicColors(true)
	f.SetTextAlign(tview.AlignCenter)
	f.SetBorderPadding(0, 0, 0, 0)
	f.refresh()
	styles.AddListener(f)

	return f
}

// StylesChanged notifies skin changed.
func (f *FKeyBar) StylesChanged(s *config.Styles) {
	f.styles = s
	f.SetBackgroundColor(s.BgColor())
	f.refresh()
}

func (f *FKeyBar) refresh() {
	f.Clear()
	fmt.Fprint(f, rk9s.FKeyLegend())
}
