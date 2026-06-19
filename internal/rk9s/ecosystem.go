// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

// Package rk9s centralizes rk9s F-key dashboards and ←/→ ecosystem tab navigation.
package rk9s

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/labels"
)

// Tab is one arrow-navigable view inside an ecosystem group.
type Tab struct {
	// Entry is the canonical index key (nav command, optionally "|labelSelector").
	Entry string
	// NavCmd is the k9s command used to open the view.
	NavCmd string
	// LabelSel is an optional Kubernetes label selector applied after navigation.
	LabelSel string
	// ShortName is the compact label shown in the tab bar.
	ShortName string
}

// Group is a related set of tabs, usually tied to one F-key hotkey.
type Group struct {
	ID          string
	FKey        string
	BarLabel    string
	HotkeyID    string
	HotkeyDesc  string
	HotkeyCmd   string
	Tabs        []Tab
}

// Hotkey describes a default rk9s F-key binding.
type Hotkey struct {
	ID          string
	ShortCut    string
	Description string
	Command     string
}

// Registry holds ecosystem groups and supports tab navigation lookups.
type Registry struct {
	groups []Group
	index  map[string]tabRef
}

type tabRef struct {
	group int
	pos   int
}

// DefaultRegistry is the built-in rk9s ecosystem navigation map.
func DefaultRegistry() *Registry {
	r := &Registry{
		groups: defaultGroups(),
		index:  make(map[string]tabRef),
	}
	for gi, grp := range r.groups {
		for pi, tab := range grp.Tabs {
			r.index[tab.Entry] = tabRef{group: gi, pos: pi}
		}
	}
	return r
}

func defaultGroups() []Group {
	return []Group{
		{
			ID: "dashboards", FKey: "F1", BarLabel: "Dash",
			HotkeyID: "rk9s-home", HotkeyDesc: "Home Dashboard", HotkeyCmd: "home",
			Tabs: tabs(
				"home", "Home",
				"rke2k3s", "RKE2/K3s",
				"etcd", "etcd",
				"k3k", "K3k",
				"kubewarden", "KW",
				"releases", "Releases",
			),
		},
		{
			ID: "longhorn", FKey: "F7", BarLabel: "LH",
			HotkeyID: "rk9s-longhorn", HotkeyDesc: "Longhorn Volumes", HotkeyCmd: "volumes.longhorn.io",
			Tabs: tabs(
				"volumes.longhorn.io", "Volumes",
				"replicas.longhorn.io", "Replicas",
				"engines.longhorn.io", "Engines",
				"nodes.longhorn.io", "LH-Nodes",
				"backupvolumes.longhorn.io", "Backups",
			),
		},
		{
			ID: "fleet", FKey: "F6", BarLabel: "Fleet",
			HotkeyID: "rk9s-fleet", HotkeyDesc: "Fleet GitRepos", HotkeyCmd: "gitrepos.fleet.cattle.io",
			Tabs: tabs(
				"gitrepos.fleet.cattle.io", "GitRepos",
				"bundledeployments.fleet.cattle.io", "BndDeploy",
				"bundles.fleet.cattle.io", "Bundles",
				"clustergroups.fleet.cattle.io", "Groups",
				"clusters.fleet.cattle.io", "Clusters",
			),
		},
		{
			ID: "rancher", FKey: "F2", BarLabel: "Rancher",
			HotkeyID: "rk9s-rancher", HotkeyDesc: "Rancher Clusters", HotkeyCmd: "clusters.management.cattle.io",
			Tabs: tabs(
				"clusters.management.cattle.io", "Clusters",
				"projects.management.cattle.io", "Projects",
				"users.management.cattle.io", "Users",
				"settings.management.cattle.io", "Settings",
				"clusterrepos.catalog.cattle.io", "Repos",
			),
		},
		{
			ID: "kubevirt", FKey: "F8", BarLabel: "VMs",
			HotkeyID: "rk9s-harvester", HotkeyDesc: "KubeVirt VMs", HotkeyCmd: "virtualmachines.kubevirt.io",
			Tabs: tabs(
				"virtualmachines.kubevirt.io", "VMs",
				"virtualmachineinstances.kubevirt.io", "VMIs",
			),
		},
		{
			ID: "distro", FKey: "F3", BarLabel: "Distro",
			HotkeyID: "rk9s-distro", HotkeyDesc: "Distro Resources", HotkeyCmd: "addons.k3s.cattle.io",
			Tabs: tabs(
				"addons.k3s.cattle.io", "Addons",
				"helmcharts.helm.cattle.io", "HelmCharts",
				"helmchartconfigs.helm.cattle.io", "HelmCfg",
				"plans.upgrade.cattle.io", "Upgrades",
			),
		},
		{
			ID: "etcd", FKey: "F4", BarLabel: "etcd",
			HotkeyID: "rk9s-etcd", HotkeyDesc: "etcd Snapshots", HotkeyCmd: "etcdsnapshots.rke.cattle.io",
			Tabs: tabs(
				"etcdsnapshots.rke.cattle.io", "Snapshots",
			),
		},
		{
			ID: "nodes", FKey: "F5", BarLabel: "Nodes",
			HotkeyID: "rk9s-nodes", HotkeyDesc: "Nodes", HotkeyCmd: "nodes",
			Tabs: []Tab{
				{Entry: "v1/nodes", NavCmd: "v1/nodes", ShortName: "All"},
				nodeTab("node-role.kubernetes.io/control-plane=true", "CP"),
				nodeTab("!node-role.kubernetes.io/control-plane", "Workers"),
				{Entry: "nodepools.management.cattle.io", NavCmd: "nodepools.management.cattle.io", ShortName: "NodePools"},
				{Entry: "machines.cluster.x-k8s.io", NavCmd: "machines.cluster.x-k8s.io", ShortName: "Machines"},
				{Entry: "machinedeployments.cluster.x-k8s.io", NavCmd: "machinedeployments.cluster.x-k8s.io", ShortName: "MachDeploy"},
			},
		},
		{
			ID: "kubewarden", FKey: "", BarLabel: "KW",
			Tabs: tabs(
				"clusteradmissionpolicies.policies.kubewarden.io", "ClusterPol",
				"admissionpolicies.policies.kubewarden.io", "Policies",
				"policyservers.policies.kubewarden.io", "PolServers",
				"policyreports.wgpolicyk8s.io", "Reports",
			),
		},
		{
			ID: "k3k", FKey: "", BarLabel: "K3k",
			Tabs: tabs(
				"clusters.k3k.io", "Clusters",
			),
		},
	}
}

func tabs(pairs ...string) []Tab {
	if len(pairs)%2 != 0 {
		panic("rk9s: tabs requires even number of arguments")
	}
	out := make([]Tab, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		entry, name := pairs[i], pairs[i+1]
		out = append(out, Tab{Entry: entry, NavCmd: entry, ShortName: name})
	}
	return out
}

func nodeTab(labelSel, shortName string) Tab {
	entry := "v1/nodes|" + labelSel
	return Tab{Entry: entry, NavCmd: "v1/nodes", LabelSel: labelSel, ShortName: shortName}
}

// ParseEntry splits "nav|label" into parts.
func ParseEntry(entry string) (navCmd, labelSel string, hasLabel bool) {
	if idx := strings.Index(entry, "|"); idx >= 0 {
		return entry[:idx], entry[idx+1:], true
	}
	return entry, "", false
}

// GVRToEntry maps a GVR path to a registry entry key.
func (r *Registry) GVRToEntry(gvr string) string {
	if _, ok := r.index[gvr]; ok {
		return gvr
	}
	parts := strings.Split(gvr, "/")
	if len(parts) == 3 {
		return parts[2] + "." + parts[0]
	}
	return gvr
}

// AliasKey resolves the active tab entry for a GVR and optional label filter.
func (r *Registry) AliasKey(gvr string, sel labels.Selector) string {
	if sel != nil && !sel.Empty() {
		selStr := sel.String()
		prefix := gvr + "|"
		for entry := range r.index {
			if !strings.HasPrefix(entry, prefix) {
				continue
			}
			_, labelPart, ok := ParseEntry(entry)
			if !ok {
				continue
			}
			parsed, err := labels.Parse(labelPart)
			if err == nil && parsed.String() == selStr {
				return entry
			}
		}
	}
	return r.GVRToEntry(gvr)
}

// InGroup reports whether an entry participates in arrow navigation.
func (r *Registry) InGroup(entry string) bool {
	ref, ok := r.index[entry]
	if !ok {
		return false
	}
	return len(r.groups[ref.group].Tabs) > 1
}

// TabBar renders the coloured ←/→ tab hint for the table title.
// Futuristic neon style with Unicode glyphs.
func (r *Registry) TabBar(entry string) string {
	ref, ok := r.index[entry]
	if !ok {
		return ""
	}
	grp := r.groups[ref.group]
	if len(grp.Tabs) < 2 {
		return ""
	}

	activeColor := "[#7dcfff::b]"
	inactiveColor := "[#565f89::-]"
	accentColor := "[#bb9af7::-]"
	reset := "[-::-]"

	var sb strings.Builder
	if grp.FKey != "" {
		sb.WriteString(fmt.Sprintf("  %s%s%s%s·%s%s", accentColor, grp.FKey, reset, inactiveColor, grp.BarLabel, reset))
	}
	sb.WriteString(fmt.Sprintf(" %s◂%s", inactiveColor, reset))
	for i, tab := range grp.Tabs {
		if i == ref.pos {
			sb.WriteString(fmt.Sprintf("%s ▸%s%s ", activeColor, tab.ShortName, reset))
		} else {
			sb.WriteString(fmt.Sprintf("%s %s %s", inactiveColor, tab.ShortName, reset))
		}
		if i < len(grp.Tabs)-1 {
			sb.WriteString(fmt.Sprintf("%s│%s", inactiveColor, reset))
		}
	}
	sb.WriteString(fmt.Sprintf("%s▸%s", inactiveColor, reset))
	return sb.String()
}

// Candidate returns the next or previous navigable tab (dir=1 forward, dir=-1 back).
// Entries failing canResolve are skipped.
func (r *Registry) Candidate(entry string, dir int, canResolve func(string) bool) (navCmd, labelSel string, ok bool) {
	if dir == 0 {
		return "", "", false
	}
	ref, ok := r.index[entry]
	if !ok {
		return "", "", false
	}
	grp := r.groups[ref.group].Tabs
	if len(grp) < 2 {
		return "", "", false
	}
	step := 1
	if dir < 0 {
		step = -1
	}
	for n := 1; n < len(grp); n++ {
		pos := ref.pos + step*n
		if step > 0 {
			pos %= len(grp)
		} else {
			pos = (pos%len(grp) + len(grp)) % len(grp)
		}
		tab := grp[pos]
		if canResolve == nil || canResolve(tab.NavCmd) {
			return tab.NavCmd, tab.LabelSel, true
		}
	}
	return "", "", false
}

// Groups returns a copy of registered ecosystem groups (for tests/introspection).
func (r *Registry) Groups() []Group {
	out := make([]Group, len(r.groups))
	copy(out, r.groups)
	return out
}

// DefaultHotkeys returns rk9s F-key bindings (plus non-ecosystem keys).
func DefaultHotkeys() []Hotkey {
	r := DefaultRegistry()
	out := []Hotkey{
		{ID: "rk9s-home", ShortCut: "F1", Description: "Home Dashboard", Command: "home"},
		{ID: "rk9s-status", ShortCut: "F9", Description: "rk9s Status", Command: "rk9s"},
		{ID: "rk9s-contexts", ShortCut: "F10", Description: "Contexts", Command: "context"},
	}
	seen := make(map[string]bool, len(out))
	for _, h := range out {
		seen[h.ShortCut] = true
	}
	for _, grp := range r.groups {
		if grp.HotkeyID == "" || seen[grp.FKey] {
			continue
		}
		out = append(out, Hotkey{
			ID:          grp.HotkeyID,
			ShortCut:    grp.FKey,
			Description: grp.HotkeyDesc,
			Command:     grp.HotkeyCmd,
		})
		seen[grp.FKey] = true
	}
	return out
}

// FKeyLegend builds the bottom-bar F-key hint string for the TUI.
// Compact style: F1·Dash F2·Rancher ... with rK9s branding at right.
func FKeyLegend() string {
	type slot struct {
		key  string
		desc string
	}
	slots := []slot{
		{"1", "Dash"},
		{"2", "Rnchr"},
		{"3", "Distr"},
		{"4", "etcd"},
		{"5", "Node"},
		{"6", "Fleet"},
		{"7", "LH"},
		{"8", "VM"},
		{"9", "Info"},
		{"0", "Ctx"},
	}
	keyColor := "[#7dcfff::-]"
	descColor := "[#565f89::-]"
	sepColor := "[#3b4261::-]"
	brandColor := "[#bb9af7::b]"
	reset := "[-::-]"

	var sb strings.Builder
	for i, s := range slots {
		if i > 0 {
			sb.WriteString(fmt.Sprintf("%s·%s", sepColor, reset))
		}
		sb.WriteString(fmt.Sprintf("%sF%s%s%s", keyColor, s.key, descColor, s.desc))
	}
	// Add rK9s branding at right
	sb.WriteString(fmt.Sprintf("  %s│%s %s[green::-]r[#7dcfff::b]K9s%s", sepColor, reset, brandColor, reset))
	return sb.String()
}
