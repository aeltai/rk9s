# rk9s Zellij Integration

Futuristic terminal layouts for running rk9s with Zellij multiplexer.

## Installation

```bash
# Copy layouts to Zellij config
mkdir -p ~/.config/zellij/layouts
cp zellij/rk9s.kdl zellij/rk9s-multi.kdl ~/.config/zellij/layouts/

# Install status script
cp zellij/k8s-status.sh ~/.local/bin/
chmod +x ~/.local/bin/k8s-status.sh
```

## Quick Start

```bash
# Simple layout
zellij -l ~/.config/zellij/layouts/rk9s.kdl

# Multi-pane power user layout
zellij -l ~/.config/zellij/layouts/rk9s-multi.kdl
```

## Layouts

### `rk9s.kdl` - Simple Layout
- **Tab 1 (⎈ rk9s)**: Main rk9s with cluster status header
- **Tab 2 (▶ kubectl)**: kubectl shell
- **Tab 3 (◉ logs)**: Pod logs viewer

### `rk9s-multi.kdl` - Multi-Pane Layout
- **Tab 1 (⎈ Cluster)**:
  - Left (70%): rk9s with status header
  - Top-right: kubectl shell
  - Bottom-right: watch panel
- **Tab 2 (◫ Debug)**: Split debug panes

## Cluster Status Script

The `k8s-status.sh` script shows live cluster info:
```
⎈ k3d-rancher-mgmt │ ◈ default │ ▣ 3 nodes │ ● 42 pods │ 19:30:00
```

Updates every 5 seconds.

## Theme

The layouts use a custom "rk9s" theme matching the neon skin:
- Transparent background
- Cyan/magenta neon accents
- High contrast text

## rk9s F-Keys in Zellij

| Key | Dashboards (←/→) |
|-----|------------------|
| F1 | Home · RKE2/K3s · etcd · K3k · KW · Releases |
| F2 | Rancher Clusters → Projects → Users → Settings |
| F3 | Distro: Addons → HelmCharts → Upgrades |
| F5 | Nodes: All → CP → Workers → Machines |

## Tips

1. **Focus rk9s**: Press `Alt+1` to focus the rk9s pane
2. **New kubectl shell**: `Ctrl+n` then type command
3. **Toggle floating pane**: `Ctrl+p f` for quick kubectl access
4. **Resize panes**: `Ctrl+n` then arrow keys
5. **Quick diagnose**: In rk9s, `Shift-H` on any pod

## Requirements

- [Zellij](https://zellij.dev/) >= 0.40
- rk9s installed at `~/.local/bin/rk9s` or in PATH
- kubectl configured
