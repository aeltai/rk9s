// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package ui_test

import (
	"strings"
	"testing"

	"github.com/derailed/k9s/internal/config"
	"github.com/derailed/k9s/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestFKeyBarLegend(t *testing.T) {
	bar := ui.NewFKeyBar(config.NewStyles())
	text := bar.GetText(true)
	// Compact format with rK9s branding
	for _, want := range []string{"F1", "Dash", "F5", "Node", "rK9s"} {
		assert.True(t, strings.Contains(text, want), "missing %q", want)
	}
}
