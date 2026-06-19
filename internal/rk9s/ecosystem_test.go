// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package rk9s_test

import (
	"strings"
	"testing"

	"github.com/derailed/k9s/internal/rk9s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
)

func TestParseEntry(t *testing.T) {
	cases := []struct {
		entry   string
		nav     string
		label   string
		hasLbl  bool
	}{
		{"v1/nodes", "v1/nodes", "", false},
		{"v1/nodes|node-role.kubernetes.io/control-plane=true", "v1/nodes", "node-role.kubernetes.io/control-plane=true", true},
		{"v1/nodes|!node-role.kubernetes.io/control-plane", "v1/nodes", "!node-role.kubernetes.io/control-plane", true},
		{"gitrepos.fleet.cattle.io", "gitrepos.fleet.cattle.io", "", false},
	}
	for _, tc := range cases {
		nav, label, has := rk9s.ParseEntry(tc.entry)
		assert.Equal(t, tc.nav, nav, tc.entry)
		assert.Equal(t, tc.label, label, tc.entry)
		assert.Equal(t, tc.hasLbl, has, tc.entry)
	}
}

func TestRegistryAliasKey(t *testing.T) {
	r := rk9s.DefaultRegistry()

	assert.Equal(t, "v1/nodes", r.AliasKey("v1/nodes", nil))
	assert.Equal(t, "v1/nodes", r.AliasKey("v1/nodes", labels.Everything()))

	cp, err := labels.Parse("node-role.kubernetes.io/control-plane=true")
	require.NoError(t, err)
	assert.Equal(t, "v1/nodes|node-role.kubernetes.io/control-plane=true", r.AliasKey("v1/nodes", cp))

	workers, err := labels.Parse("!node-role.kubernetes.io/control-plane")
	require.NoError(t, err)
	assert.Equal(t, "v1/nodes|!node-role.kubernetes.io/control-plane", r.AliasKey("v1/nodes", workers))

	assert.Equal(t, "gitrepos.fleet.cattle.io", r.AliasKey("fleet.cattle.io/v1alpha1/gitrepos", nil))
}

func TestRegistryGVRToEntry(t *testing.T) {
	r := rk9s.DefaultRegistry()
	assert.Equal(t, "gitrepos.fleet.cattle.io", r.GVRToEntry("fleet.cattle.io/v1alpha1/gitrepos"))
	assert.Equal(t, "v1/nodes", r.GVRToEntry("v1/nodes"))
}

func TestRegistryInGroup(t *testing.T) {
	r := rk9s.DefaultRegistry()
	assert.True(t, r.InGroup("v1/nodes"))
	assert.True(t, r.InGroup("v1/nodes|node-role.kubernetes.io/control-plane=true"))
	assert.False(t, r.InGroup("etcdsnapshots.rke.cattle.io"))
	assert.True(t, r.InGroup("gitrepos.fleet.cattle.io"))
}

func TestRegistryTabBar(t *testing.T) {
	r := rk9s.DefaultRegistry()

	bar := r.TabBar("v1/nodes|node-role.kubernetes.io/control-plane=true")
	assert.Contains(t, bar, "F5")
	assert.Contains(t, bar, "Nodes")
	assert.Contains(t, bar, "CP")
	assert.Contains(t, bar, "Workers")
	assert.Contains(t, bar, "◂")
	assert.Contains(t, bar, "▸")

	assert.Empty(t, r.TabBar("etcdsnapshots.rke.cattle.io"))
	assert.Empty(t, r.TabBar("unknown.resource.io"))
}

func TestRegistryCandidate(t *testing.T) {
	r := rk9s.DefaultRegistry()
	resolveAll := func(string) bool { return true }
	resolveNodesOnly := func(cmd string) bool { return cmd == "v1/nodes" }

	nav, label, ok := r.Candidate("v1/nodes", 1, resolveAll)
	assert.True(t, ok)
	assert.Equal(t, "v1/nodes", nav)
	assert.Equal(t, "node-role.kubernetes.io/control-plane=true", label)

	nav, label, ok = r.Candidate("v1/nodes|node-role.kubernetes.io/control-plane=true", 1, resolveAll)
	assert.True(t, ok)
	assert.Equal(t, "v1/nodes", nav)
	assert.Equal(t, "!node-role.kubernetes.io/control-plane", label)

	nav, label, ok = r.Candidate("v1/nodes|!node-role.kubernetes.io/control-plane", -1, resolveAll)
	assert.True(t, ok)
	assert.Equal(t, "v1/nodes", nav)
	assert.Equal(t, "node-role.kubernetes.io/control-plane=true", label)

	nav, label, ok = r.Candidate("v1/nodes|!node-role.kubernetes.io/control-plane", 1, resolveNodesOnly)
	assert.True(t, ok)
	assert.Equal(t, "v1/nodes", nav)
	assert.Empty(t, label)
}

func TestDefaultHotkeys(t *testing.T) {
	hotkeys := rk9s.DefaultHotkeys()
	byID := make(map[string]rk9s.Hotkey, len(hotkeys))
	for _, h := range hotkeys {
		byID[h.ID] = h
	}
	assert.Equal(t, "home", byID["rk9s-home"].Command)
	assert.Equal(t, "F4", byID["rk9s-etcd"].ShortCut)
	assert.Equal(t, "etcdsnapshots.rke.cattle.io", byID["rk9s-etcd"].Command)
	assert.Equal(t, "F5", byID["rk9s-nodes"].ShortCut)
	assert.Equal(t, "nodes", byID["rk9s-nodes"].Command)
	assert.Equal(t, "addons.k3s.cattle.io", byID["rk9s-distro"].Command)
}

func TestFKeyLegend(t *testing.T) {
	legend := rk9s.FKeyLegend()
	// Compact format: F1Dash, F5Node, etc. with rK9s at end
	for _, want := range []string{"F1", "Dash", "F5", "Node", "F0", "Ctx", "K9s"} {
		assert.True(t, strings.Contains(legend, want), "missing %q in %q", want, legend)
	}
}

func TestNodeTabsUniqueEntries(t *testing.T) {
	r := rk9s.DefaultRegistry()
	seen := make(map[string]string)
	for _, grp := range r.Groups() {
		for _, tab := range grp.Tabs {
			if prev, ok := seen[tab.Entry]; ok {
				t.Fatalf("duplicate tab entry %q in groups %q and current", tab.Entry, prev)
			}
			seen[tab.Entry] = grp.ID
		}
	}
}

func TestDashboardsGroup(t *testing.T) {
	r := rk9s.DefaultRegistry()
	var dashGrp *rk9s.Group
	for _, grp := range r.Groups() {
		if grp.ID == "dashboards" {
			g := grp
			dashGrp = &g
			break
		}
	}
	require.NotNil(t, dashGrp, "dashboards group should exist")
	assert.Equal(t, "F1", dashGrp.FKey)
	assert.Equal(t, "home", dashGrp.HotkeyCmd)

	// Check all dashboard tabs exist
	tabNames := make([]string, 0, len(dashGrp.Tabs))
	for _, tab := range dashGrp.Tabs {
		tabNames = append(tabNames, tab.Entry)
	}
	assert.Contains(t, tabNames, "home")
	assert.Contains(t, tabNames, "rke2k3s")
	assert.Contains(t, tabNames, "etcd")
	assert.Contains(t, tabNames, "k3k")
	assert.Contains(t, tabNames, "kubewarden")
	assert.Contains(t, tabNames, "releases")
}

func TestDashboardTabNavigation(t *testing.T) {
	r := rk9s.DefaultRegistry()
	resolveAll := func(string) bool { return true }

	// From home, next should be rke2k3s
	nav, _, ok := r.Candidate("home", 1, resolveAll)
	assert.True(t, ok)
	assert.Equal(t, "rke2k3s", nav)

	// From rke2k3s, prev should be home
	nav, _, ok = r.Candidate("rke2k3s", -1, resolveAll)
	assert.True(t, ok)
	assert.Equal(t, "home", nav)

	// From releases, next should wrap to home
	nav, _, ok = r.Candidate("releases", 1, resolveAll)
	assert.True(t, ok)
	assert.Equal(t, "home", nav)
}

func TestAllGroupsHaveTabs(t *testing.T) {
	r := rk9s.DefaultRegistry()
	for _, grp := range r.Groups() {
		assert.NotEmpty(t, grp.Tabs, "group %q should have tabs", grp.ID)
		assert.NotEmpty(t, grp.ID, "group should have ID")
	}
}

func TestHotkeyUniqueness(t *testing.T) {
	hotkeys := rk9s.DefaultHotkeys()
	seenKeys := make(map[string]string)
	seenIDs := make(map[string]bool)

	for _, h := range hotkeys {
		if prev, ok := seenKeys[h.ShortCut]; ok {
			t.Errorf("duplicate shortcut %q: %q and %q", h.ShortCut, prev, h.ID)
		}
		seenKeys[h.ShortCut] = h.ID

		if seenIDs[h.ID] {
			t.Errorf("duplicate hotkey ID %q", h.ID)
		}
		seenIDs[h.ID] = true
	}
}

func TestTabBarContainsGroupInfo(t *testing.T) {
	r := rk9s.DefaultRegistry()

	cases := []struct {
		entry    string
		wantFKey string
		wantTab  string
	}{
		{"home", "F1", "Home"},
		{"rke2k3s", "F1", "RKE2/K3s"},
		{"gitrepos.fleet.cattle.io", "F6", "GitRepos"},
		{"volumes.longhorn.io", "F7", "Volumes"},
	}

	for _, tc := range cases {
		bar := r.TabBar(tc.entry)
		if tc.wantFKey != "" {
			assert.Contains(t, bar, tc.wantFKey, "entry %q", tc.entry)
		}
		assert.Contains(t, bar, tc.wantTab, "entry %q", tc.entry)
	}
}
