// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package ui

import (
	"fmt"
	"strings"

	"github.com/derailed/k9s/internal/config"
	"github.com/derailed/tview"
)

// LogoSmall rk9s small logo — shown in the header while running.
// Minimal 2-line version for less visual noise.
var LogoSmall = []string{
	`rK9s`,
	`⎈ rancher`,
	``,
	``,
	``,
	``,
}

// LogoBig rk9s big logo for splash page — ASCII art colored as the Sudan flag.
var LogoBig = []string{
	`        _  _______      `,
	`  _ __ | |/ / _ \ ___  `,
	` | '__|| ' /(_) / __| `,
	` | |   | . \ _ \\__ \ `,
	` |_|   |_|\_\(_)/___/ `,
	`                       `,
}

// Sudan flag palette (red, white, black-as-gray, green).
const (
	sudanRed   = "#d21034"
	sudanWhite = "#ffffff"
	sudanBlack = "#7a7a7a"
	sudanGreen = "#007229"
)

// Splash represents a splash screen.
type Splash struct {
	*tview.Flex
}

// NewSplash instantiates a new splash screen with product and company info.
func NewSplash(styles *config.Styles, version string) *Splash {
	s := Splash{Flex: tview.NewFlex()}
	s.SetBackgroundColor(styles.BgColor())

	logo := tview.NewTextView()
	logo.SetDynamicColors(true)
	logo.SetTextAlign(tview.AlignCenter)
	s.layoutLogo(logo, styles)

	vers := tview.NewTextView()
	vers.SetDynamicColors(true)
	vers.SetTextAlign(tview.AlignCenter)
	s.layoutRev(vers, version, styles)

	s.SetDirection(tview.FlexRow)
	s.AddItem(logo, 10, 1, false)
	s.AddItem(vers, 1, 1, false)

	return &s
}

func (*Splash) layoutLogo(t *tview.TextView, _ *config.Styles) {
	// Color the ASCII art as the Sudan flag: red, white, black, green stripes.
	stripe := []string{sudanRed, sudanRed, sudanWhite, sudanBlack, sudanGreen, sudanGreen}
	_, _ = fmt.Fprintf(t, "%s", strings.Repeat("\n", 2))
	for i, line := range LogoBig {
		color := sudanGreen
		if i < len(stripe) {
			color = stripe[i]
		}
		_, _ = fmt.Fprintf(t, "[%s::b]%s", color, line)
		if i+1 < len(LogoBig) {
			_, _ = fmt.Fprintf(t, "\n")
		}
	}
	_, _ = fmt.Fprintf(t, "\n")
	// Title line: rK9s - aeltai (Sudan colors)
	_, _ = fmt.Fprintf(t, "[%s::b]r[%s::b]K[%s::b]9[%s::b]s [%s::]- [%s::b]aeltai\n",
		sudanRed, sudanWhite, sudanBlack, sudanGreen, sudanWhite, sudanGreen)
}

func (*Splash) layoutRev(t *tview.TextView, rev string, styles *config.Styles) {
	_, _ = fmt.Fprintf(t, "[%s::b]⎈ SUSE Rancher Ecosystem  [%s::]Rev [%s::b]%s",
		styles.Body().FgColor, styles.Body().FgColor, sudanRed, rev)
}
