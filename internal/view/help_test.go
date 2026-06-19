// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view_test

import (
	"strings"
	"testing"

	"github.com/derailed/k9s/internal"
	"github.com/derailed/k9s/internal/client"
	"github.com/derailed/k9s/internal/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelp(t *testing.T) {
	ctx := makeCtx(t)

	app := ctx.Value(internal.KeyApp).(*view.App)
	po := view.NewPod(client.PodGVR)
	require.NoError(t, po.Init(ctx))
	app.Content.Push(po)

	v := view.NewHelp(app)

	require.NoError(t, v.Init(ctx))
	assert.GreaterOrEqual(t, v.GetRowCount(), 15) // Variable based on rk9s hints
	assert.Equal(t, 10, v.GetColumnCount())       // RESOURCE+GENERAL+NAV+HOTKEYS+RK9S
	assert.Equal(t, "<a>", strings.TrimSpace(v.GetCell(1, 0).Text))
	assert.Equal(t, "Attach", strings.TrimSpace(v.GetCell(1, 1).Text))
}
