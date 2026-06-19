// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package ui_test

import (
	"testing"

	"github.com/derailed/k9s/internal/config"
	"github.com/derailed/k9s/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestNewLogoView(t *testing.T) {
	v := ui.NewLogo(config.NewStyles())
	v.Reset()

	// rK9s in Sudan flag colors, remaining lines in logo color
	const elogo = "[#d21034::b]r[#ffffff::b]K[#7a7a7a::b]9[#007229::b]s\n[#ffa500::b]⎈ rancher\n[#ffa500::b]\n[#ffa500::b]\n[#ffa500::b]\n[#ffa500::b]\n"
	assert.Equal(t, elogo, v.Logo().GetText(false))
	assert.Empty(t, v.Status().GetText(false))
}

func TestLogoStatus(t *testing.T) {
	uu := map[string]struct {
		logo, msg, e string
	}{
		"info": {
			"[#d21034::b]r[#ffffff::b]K[#7a7a7a::b]9[#007229::b]s\n[#008000::b]⎈ rancher\n[#008000::b]\n[#008000::b]\n[#008000::b]\n[#008000::b]\n",
			"blee",
			"[#ffffff::b]blee\n",
		},
		"warn": {
			"[#d21034::b]r[#ffffff::b]K[#7a7a7a::b]9[#007229::b]s\n[#c71585::b]⎈ rancher\n[#c71585::b]\n[#c71585::b]\n[#c71585::b]\n[#c71585::b]\n",
			"blee",
			"[#ffffff::b]blee\n",
		},
		"err": {
			"[#d21034::b]r[#ffffff::b]K[#7a7a7a::b]9[#007229::b]s\n[#ff0000::b]⎈ rancher\n[#ff0000::b]\n[#ff0000::b]\n[#ff0000::b]\n[#ff0000::b]\n",
			"blee",
			"[#ffffff::b]blee\n",
		},
	}

	v := ui.NewLogo(config.NewStyles())
	for n := range uu {
		k, u := n, uu[n]
		t.Run(k, func(t *testing.T) {
			switch k {
			case "info":
				v.Info(u.msg)
			case "warn":
				v.Warn(u.msg)
			case "err":
				v.Err(u.msg)
			}
			assert.Equal(t, u.logo, v.Logo().GetText(false))
			assert.Equal(t, u.e, v.Status().GetText(false))
		})
	}
}
