// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view

import (
	"github.com/derailed/k9s/internal/rk9s"
	"k8s.io/apimachinery/pkg/labels"
)

var ecosystemNav = rk9s.DefaultRegistry()

func aliasKeyForTab(gvrStr string, sel labels.Selector) string {
	return ecosystemNav.AliasKey(gvrStr, sel)
}

func crdTabHintFromAlias(aliasKey string) string {
	return ecosystemNav.TabBar(aliasKey)
}

func crdTabHint(gvrStr string) string {
	return ecosystemNav.TabBar(ecosystemNav.GVRToEntry(gvrStr))
}
