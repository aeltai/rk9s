# rK9s — SUSE Rancher Ecosystem Navigator

A fork of [k9s](https://github.com/derailed/k9s) optimized for managing SUSE Rancher ecosystems: RKE2, K3s, Rancher Manager, Longhorn, Fleet, Harvester, KubeVirt, Kubewarden, and K3k.

## Quick Start

```bash
# Build
go build -o rk9s ./

# Run
./rk9s
```

## F-Key Navigation

| Key | View | ←/→ Tabs |
|-----|------|----------|
| **F1** | Dashboards | Home · RKE2/K3s · etcd · K3k · Kubewarden · Releases |
| **F2** | Rancher | Clusters · Projects · Users · Settings · Repos |
| **F3** | Distro | Addons · HelmCharts · HelmCfg · Upgrades |
| **F4** | etcd | Snapshots |
| **F5** | Nodes | All · ControlPlane · Workers · NodePools · Machines |
| **F6** | Fleet | GitRepos · BundleDeploy · Bundles · Groups · Clusters |
| **F7** | Longhorn | Volumes · Replicas · Engines · LH-Nodes · Backups |
| **F8** | VMs | VMs · VMIs |
| **F9** | Info | rk9s status overview |
| **F10** | Contexts | Multi-context selection |

## Dashboards

Access via F1 and ←/→, or type the command:

| Command | Description |
|---------|-------------|
| `:home` | Overview: contexts, CLI tools, cluster stats, ecosystem |
| `:rke2k3s` | RKE2/K3s config, distro detection, HelmCharts, system pods |
| `:etcd` | etcd health, members, alarms, DB size |
| `:k3k` | K3k virtual clusters status |
| `:kubewarden` | Policy servers, policies, reports |
| `:releases` | GitHub releases for Rancher ecosystem projects |

## Key Shortcuts

### Global
- `?` — Help (all shortcuts)
- `Ctrl-E` — Toggle header
- `Ctrl-G` — Toggle breadcrumbs
- `←/→` — Cycle through tabs in current group
- `Shift-I` — Overview dashboard for current view

### Debug & AI (pods, deployments, nodes)
- `Shift-H` — Quick diagnosis (events, logs, conditions)
- `Shift-Q` — Deep analysis (describe, yaml, full logs)
- `Shift-A` — Generate AI prompt (copies to clipboard)

### Resource-Specific
See `?` within each view for context-specific shortcuts.

## Configuration

### Config Location

```
~/.config/rk9s/           # Linux
~/Library/Application Support/rk9s/   # macOS
```

### Files

| File | Purpose |
|------|---------|
| `config.yaml` | Main configuration |
| `hotkeys.yaml` | F-key and custom hotkey bindings |
| `skins/*.yaml` | Color themes |
| `plugins/*.yaml` | Plugin definitions |

### Sample config.yaml

```yaml
k9s:
  liveViewAutoRefresh: false
  refreshRate: 2
  maxConnRetry: 5
  readOnly: false
  noExitOnCtrlC: false
  ui:
    skin: neon           # Use the neon skin
    enableMouse: false
    headless: false      # Set true to hide header
    logoless: false      # Set true to hide logo
    crumbsless: false    # Set true to hide breadcrumbs
    noIcons: false
  logger:
    tail: 100
    buffer: 5000
    sinceSeconds: -1
  thresholds:
    cpu:
      critical: 90
      warn: 70
    memory:
      critical: 90
      warn: 70
```

### Environment Variables

```bash
export K9S_EDITOR="nvim"        # Editor for :edit commands
export EDITOR="nvim"            # Fallback editor
export KUBE_EDITOR="nvim"       # Kubernetes-specific editor
```

## Plugins

### Included Plugins

| Plugin | Scopes | Key Shortcuts |
|--------|--------|---------------|
| etcd | nodes | Shift-E (health), Shift-M (members), Shift-A (alarms) |
| rke2-k3s | nodes | Shift-C (config), Shift-D (services), Shift-P (crictl) |
| longhorn | volumes, replicas | Shift-I (overview), Shift-S (snapshots) |
| fleet | gitrepos, bundles | Shift-I (overview), Shift-F (force sync) |
| rancher | clusters | Shift-I (overview), Shift-K (kubeconfig) |
| kubewarden | policies | Shift-I (overview), Shift-M (mode toggle) |
| k3k | clusters | Shift-I (overview), Shift-K (kubeconfig), Shift-S (scale) |
| k8s-debug | pods, deploys | Shift-H (diagnose), Shift-Q (deep), Shift-A (AI prompt) |

### Custom Plugins

Create `~/.config/rk9s/plugins/my-plugin.yaml`:

```yaml
plugins:
  my-command:
    shortCut: Shift-X
    description: My custom command
    scopes:
      - pods
    command: bash
    background: false
    args:
      - -c
      - |
        echo "Pod: $NAME"
        echo "Namespace: $NAMESPACE"
        kubectl describe pod $NAME -n $NAMESPACE
```

## Skins

### Included Skins

- `neon` — Futuristic dark theme with cyan/magenta accents (default for rk9s)

### Using a Skin

In `config.yaml`:

```yaml
k9s:
  ui:
    skin: neon
```

Or place skin file in `~/.config/rk9s/skins/myskin.yaml` and set `skin: myskin`.

## Multi-Context Mode

1. Press **F10** to open contexts
2. Press **Space** to toggle context selection
3. Press **Ctrl-A** to select all
4. Dashboards will show data from all selected contexts

## Zellij Integration

For a multi-pane experience with status bar:

```bash
cd zellij/
zellij --layout rk9s.kdl
```

Or for multi-context layout:

```bash
zellij --layout rk9s-multi.kdl
```

## Building

```bash
# Standard build
go build -o rk9s ./

# With version info
go build -ldflags "-X main.version=v0.1.0" -o rk9s ./

# Install to PATH
go build -o ~/.local/bin/rk9s ./
```

## Requirements

- Go 1.21+
- kubectl configured with cluster access

### Optional CLIs

For full functionality, install:

- `helm` — Helm chart management
- `rancher` — Rancher CLI
- `fleet` — Fleet CLI
- `virtctl` — KubeVirt CLI
- `longhornctl` — Longhorn CLI
- `kwctl` — Kubewarden CLI
- `etcdctl` — etcd CLI
- `crictl` — Container runtime CLI

## Architecture

```
internal/
├── rk9s/
│   └── ecosystem.go     # F-key groups, tab navigation, hotkey registry
├── config/
│   ├── templates/       # Default hotkeys, aliases
│   └── default_plugins/ # Bundled plugin YAMLs
├── ui/
│   ├── fkeybar.go      # Bottom F-key legend bar
│   ├── logo.go         # rK9s logo component
│   └── indicator.go    # Top status bar
└── view/
    ├── app.go          # Dashboard implementations
    ├── browser.go      # Table view + navigation
    └── crd_groups.go   # Ecosystem adapter
```

## License

Apache-2.0 (same as upstream k9s)
