// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package view

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/adrg/xdg"
	"github.com/cenkalti/backoff/v4"
	"github.com/derailed/k9s/internal"
	"github.com/derailed/k9s/internal/client"
	"github.com/derailed/k9s/internal/config"
	"github.com/derailed/k9s/internal/model"
	"github.com/derailed/k9s/internal/slogs"
	"github.com/derailed/k9s/internal/ui"
	"github.com/derailed/k9s/internal/ui/dialog"
	"github.com/derailed/k9s/internal/view/cmd"
	"github.com/derailed/k9s/internal/vul"
	"github.com/derailed/k9s/internal/watch"
	"github.com/derailed/tcell/v2"
	"github.com/derailed/tview"
)

// ExitStatus indicates UI exit conditions.
var ExitStatus = ""

const (
	splashDelay      = 1 * time.Second
	clusterRefresh   = 15 * time.Second
	clusterInfoWidth = 50
	clusterInfoPad   = 15
)

// App represents an application view.
type App struct {
	version string
	*ui.App
	Content       *PageStack
	command       *Command
	factory       *watch.Factory
	cancelFn      context.CancelFunc
	clusterModel  *model.ClusterInfo
	cmdHistory    *model.History
	filterHistory *model.History
	conRetry      int32
	showHeader    bool
	showLogo      bool
	showCrumbs    bool
}

// NewApp returns a K9s app instance.
func NewApp(cfg *config.Config) *App {
	a := App{
		App:           ui.NewApp(cfg, cfg.K9s.ActiveContextName()),
		cmdHistory:    model.NewHistory(model.MaxHistory),
		filterHistory: model.NewHistory(model.MaxHistory),
		Content:       NewPageStack(),
	}
	a.ReloadStyles()

	a.Views()["statusIndicator"] = ui.NewStatusIndicator(a.App, a.Styles)
	a.Views()["clusterInfo"] = NewClusterInfo(&a)

	return &a
}

// ReloadStyles reloads skin file.
func (a *App) ReloadStyles() {
	a.RefreshStyles(a)
}

// UpdateClusterInfo updates clusterInfo panel
func (a *App) UpdateClusterInfo() {
	if a.factory != nil {
		a.clusterModel.Reset(a.factory)
	}
}

// ConOK checks the connection is cool, returns false otherwise.
func (a *App) ConOK() bool {
	return atomic.LoadInt32(&a.conRetry) == 0
}

// Init initializes the application.
func (a *App) Init(version string, _ int) error {
	a.version = model.NormalizeVersion(version)

	ctx := context.WithValue(context.Background(), internal.KeyApp, a)
	if err := a.Content.Init(ctx); err != nil {
		return err
	}
	a.Content.AddListener(a.Crumbs())
	a.Content.AddListener(a.Menu())

	a.App.Init()
	a.SetInputCapture(a.keyboard)
	a.bindKeys()

	// Allow initialization even without a valid connection
	// We'll fall back to context view in defaultCmd
	if a.Conn() != nil {
		ns := a.Config.ActiveNamespace()
		a.factory = watch.NewFactory(a.Conn())
		a.initFactory(ns)

		a.clusterModel = model.NewClusterInfo(a.factory, a.version, a.Config.K9s)
		a.clusterModel.AddListener(a.clusterInfo())
		a.clusterModel.AddListener(a.statusIndicator())
		if a.Conn().ConnectionOK() {
			a.clusterModel.Refresh()
			a.clusterInfo().Init()
		}
	}

	a.command = NewCommand(a)
	if err := a.command.Init(a.Config.ContextAliasesPath()); err != nil {
		return err
	}
	a.CmdBuff().SetSuggestionFn(a.suggestCommand())

	a.layout(ctx)
	a.initSignals()

	if a.Config.K9s.ImageScans.Enable {
		a.initImgScanner(version)
	}
	a.ReloadStyles()

	return nil
}

func (*App) stopImgScanner() {
	if vul.ImgScanner != nil {
		vul.ImgScanner.Stop()
	}
}

func (a *App) clearHistory() {
	a.cmdHistory.Clear()
	a.filterHistory.Clear()
}

func (a *App) initImgScanner(version string) {
	defer func(t time.Time) {
		slog.Debug("Scanner init time", slogs.Elapsed, time.Since(t))
	}(time.Now())

	vul.ImgScanner = vul.NewImageScanner(a.Config.K9s.ImageScans, slog.Default())
	go vul.ImgScanner.Init("k9s", version)
}

func (a *App) layout(ctx context.Context) {
	flash := ui.NewFlash(a.App)
	go flash.Watch(ctx, a.Flash().Channel())

	main := tview.NewFlex().SetDirection(tview.FlexRow)
	main.AddItem(a.statusIndicator(), 1, 1, false)
	main.AddItem(a.Content, 0, 10, true)
	main.AddItem(ui.NewFKeyBar(a.Styles), 1, 1, false)
	if !a.Config.K9s.IsCrumbsless() {
		main.AddItem(a.Crumbs(), 1, 1, false)
	}
	main.AddItem(flash, 1, 1, false)

	a.Main.AddPage("main", main, true, false)
	a.toggleHeader(!a.Config.K9s.IsHeadless(), !a.Config.K9s.IsLogoless())
	if !a.Config.K9s.IsSplashless() {
		a.Main.AddPage("splash", ui.NewSplash(a.Styles, a.version), true, true)
	}
}

func (*App) initSignals() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP)

	go func(sig chan os.Signal) {
		<-sig
		os.Exit(0)
	}(sig)
}

func (a *App) suggestCommand() model.SuggestionFunc {
	contextNames, err := a.contextNames()
	if err != nil {
		slog.Error("Failed to list contexts", slogs.Error, err)
	}

	return func(s string) (entries sort.StringSlice) {
		if s == "" {
			if a.cmdHistory.Empty() {
				return
			}
			return a.cmdHistory.List()
		}

		ls := strings.ToLower(s)
		for alias := range maps.Keys(a.command.alias.Alias) {
			if suggest, ok := cmd.ShouldAddSuggest(ls, alias); ok {
				entries = append(entries, suggest)
			}
		}

		namespaceNames, err := a.factory.Client().ValidNamespaceNames()
		if err != nil {
			slog.Error("Failed to obtain list of namespaces", slogs.Error, err)
		}
		entries = append(entries, cmd.SuggestSubCommand(s, namespaceNames, contextNames)...)
		if len(entries) == 0 {
			return nil
		}
		entries.Sort()
		return
	}
}

func (a *App) contextNames() ([]string, error) {
	// Return empty list if no factory
	if a.factory == nil {
		return []string{}, nil
	}
	contexts, err := a.factory.Client().Config().Contexts()
	if err != nil {
		return nil, err
	}
	contextNames := make([]string, 0, len(contexts))
	for ctxName := range contexts {
		contextNames = append(contextNames, ctxName)
	}

	return contextNames, nil
}

func (a *App) keyboard(evt *tcell.EventKey) *tcell.EventKey {
	if k, ok := a.HasAction(ui.AsKey(evt)); ok && !a.Content.IsTopDialog() {
		return k.Action(evt)
	}

	return evt
}

func (a *App) bindKeys() {
	a.AddActions(ui.NewKeyActionsFromMap(ui.KeyMap{
		tcell.KeyCtrlE:     ui.NewSharedKeyAction("ToggleHeader", a.toggleHeaderCmd, false),
		tcell.KeyCtrlG:     ui.NewSharedKeyAction("ToggleCrumbs", a.toggleCrumbsCmd, false),
		ui.KeyHelp:         ui.NewSharedKeyAction("Help", a.helpCmd, false),
		ui.KeyLeftBracket:  ui.NewSharedKeyAction("Go Back", a.previousCommand, false),
		ui.KeyRightBracket: ui.NewSharedKeyAction("Go Forward", a.nextCommand, false),
		ui.KeyDash:         ui.NewSharedKeyAction("Last View", a.lastCommand, false),
		tcell.KeyCtrlA:     ui.NewSharedKeyAction("Aliases", a.aliasCmd, false),
		tcell.KeyEnter:     ui.NewKeyAction("Goto", a.gotoCmd, false),
		tcell.KeyCtrlC:     ui.NewKeyAction("Quit", a.quitCmd, false),
	}))
	a.loadAppHotKeys()
}

func (a *App) loadAppHotKeys() {
	hh := config.NewHotKeys()
	if err := hh.Load(a.Config.ContextHotkeysPath()); err != nil {
		return
	}
	for _, hk := range hh.HotKey {
		key, err := asKey(hk.ShortCut)
		if err != nil {
			continue
		}
		command := hk.Command
		a.AddActions(ui.NewKeyActionsFromMap(ui.KeyMap{
			key: ui.NewSharedKeyAction(hk.Description, func(*tcell.EventKey) *tcell.EventKey {
				a.gotoResource(command, "", true, true)
				return nil
			}, false),
		}))
	}
}

// ActiveView returns the currently active view.
func (a *App) ActiveView() model.Component {
	return a.Content.GetPrimitive("main").(model.Component)
}

func (a *App) toggleHeader(header, logo bool) {
	a.showHeader, a.showLogo = header, logo
	flex, ok := a.Main.GetPrimitive("main").(*tview.Flex)
	if !ok {
		slog.Error("Expecting flex view main panel. Exiting!")
		os.Exit(1)
	}
	if a.showHeader {
		flex.RemoveItemAtIndex(0)
		flex.AddItemAtIndex(0, a.buildHeader(), 8, 1, false)
	} else {
		flex.RemoveItemAtIndex(0)
		flex.AddItemAtIndex(0, a.statusIndicator(), 1, 1, false)
	}
}

func (a *App) toggleCrumbs(flag bool) {
	a.showCrumbs = flag
	flex, ok := a.Main.GetPrimitive("main").(*tview.Flex)
	if !ok {
		slog.Error("Expecting valid flex view main panel. Exiting!")
		os.Exit(1)
	}
	if a.showCrumbs {
		if _, ok := flex.ItemAt(2).(*ui.Crumbs); !ok {
			flex.AddItemAtIndex(2, a.Crumbs(), 1, 1, false)
		}
	} else {
		flex.RemoveItemAtIndex(2)
	}
}

func (a *App) buildHeader() tview.Primitive {
	header := tview.NewFlex()
	header.SetBackgroundColor(a.Styles.BgColor())
	header.SetDirection(tview.FlexColumn)
	if !a.showHeader {
		return header
	}

	clWidth := clusterInfoWidth
	if a.Conn() != nil && a.Conn().ConnectionOK() {
		n, err := a.Conn().Config().CurrentClusterName()
		if err == nil {
			size := len(n) + clusterInfoPad
			if size > clWidth {
				clWidth = size
			}
		}
	}
	header.AddItem(a.clusterInfo(), clWidth, 1, false)
	header.AddItem(a.Menu(), 0, 1, false)

	if a.showLogo {
		header.AddItem(a.Logo(), 26, 1, false)
	}

	return header
}

// Halt stop the application event loop.
func (a *App) Halt() {
	if a.cancelFn != nil {
		a.cancelFn()
		a.cancelFn = nil
	}
}

// Resume restarts the app event loop.
func (a *App) Resume() {
	var ctx context.Context
	ctx, a.cancelFn = context.WithCancel(context.Background())

	go a.clusterUpdater(ctx)

	if a.Config.K9s.UI.Reactive {
		if err := a.ConfigWatcher(ctx, a); err != nil {
			slog.Warn("ConfigWatcher failed", slogs.Error, err)
		}
		if err := a.SkinsDirWatcher(ctx, a); err != nil {
			slog.Warn("SkinsWatcher failed", slogs.Error, err)
		}
		if err := a.CustomViewsWatcher(ctx, a); err != nil {
			slog.Warn("CustomView watcher failed", slogs.Error, err)
		}
	}
}

func (a *App) clusterUpdater(ctx context.Context) {
	if a.Conn() == nil || !a.Conn().ConnectionOK() || a.factory == nil || a.clusterModel == nil {
		slog.Debug("Skipping cluster updater - no valid connection")
		return
	}

	if err := a.refreshCluster(ctx); err != nil {
		slog.Error("Cluster updater failed!", slogs.Error, err)
		return
	}

	bf := model.NewExpBackOff(ctx, clusterRefresh, 2*time.Minute)
	delay := clusterRefresh
	for {
		select {
		case <-ctx.Done():
			slog.Debug("ClusterInfo updater canceled!")
			return
		case <-time.After(delay):
			if err := a.refreshCluster(ctx); err != nil {
				slog.Error("Cluster updates failed. Giving up ;(", slogs.Error, err)
				if delay = bf.NextBackOff(); delay == backoff.Stop {
					a.BailOut(1)
					return
				}
			} else {
				bf.Reset()
				delay = clusterRefresh
			}
		}
	}
}

func (a *App) refreshCluster(context.Context) error {
	if a.Conn() == nil || a.factory == nil || a.clusterModel == nil {
		return nil
	}

	c := a.Content.Top()
	if ok := a.Conn().CheckConnectivity(); ok {
		if atomic.LoadInt32(&a.conRetry) > 0 {
			atomic.StoreInt32(&a.conRetry, 0)
			a.Status(model.FlashInfo, "K8s connectivity OK")
			if c != nil {
				c.Start()
			}
		} else {
			a.ClearStatus(true)
		}
		a.factory.ValidatePortForwards()
	} else if c != nil {
		atomic.AddInt32(&a.conRetry, 1)
		c.Stop()
	}

	count, maxConnRetry := atomic.LoadInt32(&a.conRetry), a.Config.K9s.MaxConnRetry
	if count >= maxConnRetry {
		slog.Error("Conn check failed. Bailing out!",
			slogs.Retry, count,
			slogs.MaxRetries, maxConnRetry,
		)
		ExitStatus = fmt.Sprintf("Lost K8s connection (%d). Bailing out!", count)
		a.BailOut(1)
	}
	if count > 0 {
		a.Status(model.FlashWarn, fmt.Sprintf("Dial K8s Toast [%d/%d]", count, maxConnRetry))
		return fmt.Errorf("conn check failed (%d/%d)", count, maxConnRetry)
	}

	// Reload alias
	go func() {
		if err := a.command.Reset(a.Config.ContextAliasesPath(), false); err != nil {
			slog.Warn("Command reset failed", slogs.Error, err)
			a.QueueUpdateDraw(func() {
				a.Logo().Warn("Aliases load failed!")
			})
		}
	}()
	// Update cluster info
	a.clusterModel.Refresh()

	return nil
}

func (a *App) switchNS(ns string) error {
	if a.Config.ActiveNamespace() == ns {
		return nil
	}
	if ns == client.ClusterScope {
		ns = client.BlankNamespace
	}
	if err := a.Config.SetActiveNamespace(ns); err != nil {
		return err
	}

	return a.factory.SetActiveNS(ns)
}

func (a *App) switchContext(ci *cmd.Interpreter, force bool) error {
	contextName, ok := ci.HasContext()
	if (!ok || a.Config.ActiveContextName() == contextName) && !force {
		return nil
	}

	a.Halt()
	defer a.Resume()
	{
		a.Config.Reset()
		ct, err := a.Config.ActivateContext(contextName)
		if err != nil {
			return err
		}
		if cns, ok := ci.NSArg(); ok {
			ct.Namespace.Active = cns
		}

		p := cmd.NewInterpreter(a.Config.ActiveView())
		p.ResetContextArg()
		if p.IsContextCmd() {
			a.Config.SetActiveView(client.PodGVR.String())
		}
		ns := a.Config.ActiveNamespace()
		if !a.Conn().IsValidNamespace(ns) {
			slog.Warn("Unable to validate namespace", slogs.Namespace, ns)
			if err := a.Config.SetActiveNamespace(ns); err != nil {
				return err
			}
		}
		a.Flash().Infof("Using %q namespace", ns)

		if err := a.Config.Save(true); err != nil {
			slog.Error("Fail to save config to disk", slogs.Subsys, "config", slogs.Error, err)
		}

		if a.factory == nil && a.Conn() != nil {
			a.factory = watch.NewFactory(a.Conn())
			a.clusterModel = model.NewClusterInfo(a.factory, a.version, a.Config.K9s)
			a.clusterModel.AddListener(a.clusterInfo())
			a.clusterModel.AddListener(a.statusIndicator())
		}

		if a.factory != nil {
			a.initFactory(ns)
		}

		if err := a.command.Reset(a.Config.ContextAliasesPath(), true); err != nil {
			return err
		}

		slog.Debug("Switching Context",
			slogs.Context, contextName,
			slogs.Namespace, ns,
			slogs.View, a.Config.ActiveView(),
		)
		a.Flash().Infof("Switching context to %q::%q", contextName, ns)
		a.ReloadStyles()
		a.gotoResource(a.Config.ActiveView(), "", true, true)

		if a.clusterModel != nil {
			a.clusterModel.Reset(a.factory)
		}
	}

	return nil
}

func (a *App) initFactory(ns string) {
	a.factory.Terminate()
	a.factory.Start(ns)
}

// BailOut exists the application.
func (a *App) BailOut(exitCode int) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("Bailout failed", slogs.Error, err)
		}
	}()

	if err := nukeK9sShell(a); err != nil {
		slog.Error("Unable to nuke k9s shell pod", slogs.Error, err)
	}

	a.stopImgScanner()
	a.factory.Terminate()
	a.App.BailOut(exitCode)
}

// Run starts the application loop.
func (a *App) Run() error {
	a.Resume()

	go func() {
		if !a.Config.K9s.IsSplashless() {
			<-time.After(splashDelay)
		}
		a.QueueUpdateDraw(func() {
			a.Main.SwitchToPage("main")
			if a.CmdBuff().IsActive() {
				a.SetFocus(a.Prompt())
			}
		})
		<-time.After(500 * time.Millisecond)
		a.QueueUpdateDraw(func() {
			a.showRk9sStatus()
		})
	}()

	if err := a.command.defaultCmd(true); err != nil {
		return err
	}
	a.SetRunning(true)
	if err := a.Application.Run(); err != nil {
		return err
	}

	return nil
}

func (a *App) showRk9sStatus() {
	clis := []struct {
		name string
		bin  string
	}{
		{"rancher", "rancher"},
		{"virtctl", "virtctl"},
		{"longhornctl", "longhornctl"},
		{"kwctl", "kwctl"},
		{"fleet", "fleet"},
	}

	var found, missing []string
	for _, c := range clis {
		if _, err := exec.LookPath(c.bin); err == nil {
			found = append(found, c.name)
		} else {
			missing = append(missing, c.name)
		}
	}

	sel, _ := config.LoadSelectedContexts()
	parts := []string{fmt.Sprintf("rk9s CLIs: %s", strings.Join(found, ", "))}
	if len(found) == 0 {
		parts = []string{"rk9s: no optional CLIs found"}
	}
	if len(missing) > 0 {
		parts = append(parts, fmt.Sprintf("missing: %s", strings.Join(missing, ", ")))
	}
	if len(sel) > 0 {
		parts = append(parts, fmt.Sprintf("%d context(s) selected", len(sel)))
	}
	msg := strings.Join(parts, " | ")

	if len(found) > 0 {
		a.Flash().Info(msg)
	} else {
		a.Flash().Warn(msg)
	}
}

// Status reports a new app status for display.
func (a *App) Status(l model.FlashLevel, msg string) {
	a.QueueUpdateDraw(func() {
		if a.showHeader {
			a.setLogo(l, msg)
		} else {
			a.setIndicator(l, msg)
		}
	})
}

// IsBenchmarking check if benchmarks are active.
func (a *App) IsBenchmarking() bool {
	return a.Logo().IsBenchmarking()
}

// ClearStatus reset logo back to normal.
func (a *App) ClearStatus(flash bool) {
	a.QueueUpdate(func() {
		a.Logo().Reset()
		if flash {
			a.Flash().Clear()
		}
	})
}

func (a *App) setLogo(l model.FlashLevel, msg string) {
	switch l {
	case model.FlashErr:
		a.Logo().Err(msg)
	case model.FlashWarn:
		a.Logo().Warn(msg)
	case model.FlashInfo:
		a.Logo().Info(msg)
	default:
		a.Logo().Reset()
	}
}

func (a *App) setIndicator(l model.FlashLevel, msg string) {
	switch l {
	case model.FlashErr:
		a.statusIndicator().Err(msg)
	case model.FlashWarn:
		a.statusIndicator().Warn(msg)
	case model.FlashInfo:
		a.statusIndicator().Info(msg)
	default:
		a.statusIndicator().Reset()
	}
}

// PrevCmd pops the command stack.
func (a *App) PrevCmd(*tcell.EventKey) *tcell.EventKey {
	if !a.Content.IsLast() {
		a.Content.Pop()
	}

	return nil
}

func (a *App) toggleHeaderCmd(evt *tcell.EventKey) *tcell.EventKey {
	if a.Prompt().InCmdMode() {
		return evt
	}

	a.QueueUpdateDraw(func() {
		a.showHeader = !a.showHeader
		a.toggleHeader(a.showHeader, a.showLogo)
	})

	return nil
}

func (a *App) toggleCrumbsCmd(evt *tcell.EventKey) *tcell.EventKey {
	if a.Prompt().InCmdMode() {
		return evt
	}

	a.QueueUpdateDraw(func() {
		a.showCrumbs = !a.showCrumbs
		a.toggleCrumbs(a.showCrumbs)
	})

	return nil
}

func (a *App) gotoCmd(evt *tcell.EventKey) *tcell.EventKey {
	if a.CmdBuff().IsActive() && !a.CmdBuff().Empty() {
		a.gotoResource(a.GetCmd(), "", true, true)
		a.ResetCmd()
		return nil
	}

	return evt
}

func (a *App) cowCmd(msg string) {
	d := a.Styles.Dialog()
	dialog.ShowError(&d, a.Content.Pages, msg)
}

func pluginDir(appName string) string {
	dir, err := xdg.DataFile(filepath.Join(appName, "plugins"))
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".local", "share", appName, "plugins")
	}
	return dir
}

func (a *App) runDashScript(title, subject, script string) {
	a.Flash().Infof("Loading %s dashboard...", title)
	go func() {
		out, err := oneShoot(context.Background(), &shellOpts{
			binary: "bash",
			args:   []string{"-c", script},
		})
		if err != nil {
			out = fmt.Sprintf("Error: %s\n\n%s", err, out)
		}
		a.QueueUpdateDraw(func() {
			details := NewDetails(a, title, subject, contentTXT, true).Update(out)
			if e := a.inject(details, false); e != nil {
				a.Flash().Err(e)
			}
		})
	}()
}

func (a *App) rk9sCmd() {
	sel, _ := config.LoadSelectedContexts()
	ctxInfo := "(none) — use :contexts then Space to select"
	if len(sel) > 0 {
		ctxInfo = strings.Join(sel, ", ")
	}
	script := fmt.Sprintf(`
echo '╔═══════════════════════════════════════════════════════════╗'
echo '║                    rk9s Status                             ║'
echo '║           SUSE Rancher Ecosystem Navigator                 ║'
echo '╚═══════════════════════════════════════════════════════════╝'
echo ''
echo '┌─ CLI Tools ───────────────────────────────────────────────┐'
for cli in kubectl helm rancher fleet virtctl longhornctl kwctl etcdctl k3k crictl jq; do
  p=$(command -v "$cli" 2>/dev/null)
  if [ -n "$p" ]; then
    ver=$("$cli" version --client 2>/dev/null || "$cli" --version 2>/dev/null || "$cli" version 2>/dev/null | head -1 || echo "ok")
    printf '│ ✓ %%-12s %%s\n' "$cli" "$(echo "$ver" | head -c 40)"
  else
    printf '│ ✗ %%-12s (not found)\n' "$cli"
  fi
done
echo '└──────────────────────────────────────────────────────────┘'
echo ''
echo '┌─ F-Key Navigation ───────────────────────────────────────┐'
echo '│  F1  ◈ Dashboards   F2  ⎈ Rancher     F3  ⚙ Distro      │'
echo '│  F4  ◉ etcd snaps   F5  ▣ Nodes       F6  ⇉ Fleet       │'
echo '│  F7  ▤ Longhorn     F8  ◫ VMs         F9  ℹ Info        │'
echo '│  F10 ⊛ Contexts     ←/→ Cycle tabs   Shift-I Overview   │'
echo '└──────────────────────────────────────────────────────────┘'
echo ''
echo '┌─ Dashboards (:command) ──────────────────────────────────┐'
echo '│  :home        Home dashboard (F1)                        │'
echo '│  :etcd        etcd health, members, alarms               │'
echo '│  :rke2k3s     RKE2/K3s cluster config                    │'
echo '│  :k3k         K3k virtual clusters                       │'
echo '│  :kubewarden  Kubewarden policy status                   │'
echo '│  :releases    GitHub releases (Rancher/Harvester/LH)     │'
echo '│  :rk9s        This status view (F9)                      │'
echo '└──────────────────────────────────────────────────────────┘'
echo ''
echo '┌─ Quick Resources ────────────────────────────────────────┐'
echo '│  :clusters.management.cattle.io    Rancher clusters      │'
echo '│  :gitrepos.fleet.cattle.io         Fleet GitRepos        │'
echo '│  :volumes.longhorn.io              Longhorn volumes      │'
echo '│  :virtualmachines.kubevirt.io      KubeVirt VMs          │'
echo '│  :clusteradmissionpolicies         Kubewarden policies   │'
echo '│  :clusters.k3k.io                  K3k clusters          │'
echo '│  :etcdsnapshots.rke.cattle.io      etcd snapshots        │'
echo '└──────────────────────────────────────────────────────────┘'
echo ''
echo '┌─ AI & Debug Tools ─────────────────────────────────────────┐'
echo '│  Shift-H   Quick diagnosis (events, logs, conditions)     │'
echo '│  Shift-Q   Deep analysis (describe, yaml, full logs)      │'
echo '│  Shift-A   Generate AI prompt (copies to clipboard)       │'
echo '│  Editor:   ${K9S_EDITOR:-${EDITOR:-vi}} (set K9S_EDITOR)  │'
echo '└──────────────────────────────────────────────────────────┘'
echo ''
echo '┌─ Multi-Context (F10 / :contexts) ────────────────────────┐'
echo '│  Space      Toggle context selection                     │'
echo '│  Ctrl-A     Select all contexts                          │'
echo '│  Selected:  %s'
echo '└──────────────────────────────────────────────────────────┘'
echo ''
echo '┌─ Plugins (%s) ─┐'
pdir="%s"
if [ -d "$pdir" ]; then
  count=$(ls -1 "$pdir"/*.yaml 2>/dev/null | wc -l | tr -d ' ')
  echo "│  $count plugin files loaded"
  echo "│  Key plugins: etcd, longhorn, rancher, fleet, kubewarden, k3k"
else
  echo '│  (plugin directory not found)'
fi
echo '└──────────────────────────────────────────────────────────┘'
echo ''
echo '  Press ? for full help  │  Shift-I for view overview'
`, ctxInfo, config.AppName, pluginDir(config.AppName))
	a.runDashScript("rk9s", ctxInfo, script)
}

func (a *App) dashContexts() ([]string, string) {
	sel, _ := config.LoadSelectedContexts()
	if len(sel) < 2 {
		ctx := a.Config.K9s.ActiveContextName()
		return []string{ctx}, ctx
	}
	return sel, strings.Join(sel, ", ")
}

func ctxListArg(ctxs []string) string {
	return strings.Join(ctxs, " ")
}

func mcKubectl(ctxs []string, args string) string {
	if len(ctxs) == 1 {
		return fmt.Sprintf("kubectl %s --context %s 2>/dev/null", args, ctxs[0])
	}
	var b strings.Builder
	b.WriteString("for _ctx in")
	for _, c := range ctxs {
		b.WriteString(" " + c)
	}
	b.WriteString("; do\n")
	b.WriteString("  echo \"  [$_ctx]\"\n")
	b.WriteString(fmt.Sprintf("  kubectl %s --context $_ctx 2>/dev/null || echo '    (unavailable)'\n", args))
	b.WriteString("  echo ''\n")
	b.WriteString("done\n")
	return b.String()
}

func (a *App) rk9sHomeDashboard() {
	ctxs, subject := a.dashContexts()
	script := fmt.Sprintf(`
echo '╔══════════════════════════════════════════════════════╗'
echo '║              rk9s Home Dashboard                     ║'
echo '╚══════════════════════════════════════════════════════╝'
echo ''
echo '=== Contexts ==='
echo '  Active:   %s'
echo '  Selected: %s'
echo ''
echo '=== Cluster Info ==='
%s
echo ''
echo '=== CLI Tools ==='
for tool in kubectl helm longhornctl virtctl fleet rancher kwctl etcdctl crictl; do
  if command -v "$tool" >/dev/null 2>&1; then
    ver=$("$tool" version --short 2>/dev/null || "$tool" --version 2>/dev/null || "$tool" version 2>/dev/null | head -1)
    printf '  ✓ %%-14s %%s\n' "$tool" "$ver"
  else
    printf '  ✗ %%-14s (not installed)\n' "$tool"
  fi
done
echo ''
echo '=== Distribution ==='
%s
echo ''
echo '=== Cluster Stats ==='
%s
echo ''
echo '=== Ecosystem Components ==='
%s
echo ''
echo '=== F-Key Navigation ==='
echo '  F1  ◈ Dashboards (←/→ Home·RKE2/K3s·etcd·K3k·KW·Releases)'
echo '  F2  ⎈ Rancher      F3  ⚙ Distro       F4  ◉ etcd snaps'
echo '  F5  ▣ Nodes        F6  ⇉ Fleet        F7  ▤ Longhorn'
echo '  F8  ◫ VMs          F9  ℹ Info         F10 ⊛ Contexts'
echo ''
echo '  Shift-I  Overview dashboard (within any view)'
echo '  ?        Full help and plugin shortcuts'
echo ''
echo '=== Dashboards ==='
echo '  :home        Home (F1)           :etcd       etcd health'
echo '  :rke2k3s     RKE2/K3s config     :k3k        K3k clusters'
echo '  :kubewarden  Policy status       :releases   GitHub releases'
echo ''
echo '=== Quick Resources ==='
echo '  :pods        :deploy      :svc           :ns'
echo '  :clusters.management.cattle.io           Rancher'
echo '  :gitrepos.fleet.cattle.io                Fleet'
echo '  :volumes.longhorn.io                     Longhorn'
echo '  :virtualmachines.kubevirt.io             VMs'
echo '  :clusteradmissionpolicies                Kubewarden'
echo '  :clusters.k3k.io                         K3k'
`,
		a.Config.K9s.ActiveContextName(),
		subject,
		mcKubectl(ctxs, `get nodes -o custom-columns='NAME:.metadata.name,STATUS:.status.conditions[?(@.type=="Ready")].status,ROLES:.metadata.labels.node-role\.kubernetes\.io/control-plane,VERSION:.status.nodeInfo.kubeletVersion,OS:.status.nodeInfo.osImage' 2>/dev/null || echo '  (unavailable)'`),
		a.distroDetectScript(ctxs),
		a.clusterStatsScript(ctxs),
		a.ecosystemDetectScript(ctxs),
	)
	a.runDashScript("home", subject, script)
}

func (a *App) distroDetectScript(ctxs []string) string {
	var b strings.Builder
	for _, ctx := range ctxs {
		b.WriteString(fmt.Sprintf(`
_ctx="%s"
echo "  [$_ctx]"
_ver=$(kubectl --context "$_ctx" get nodes -o jsonpath='{.items[0].status.nodeInfo.kubeletVersion}' 2>/dev/null)
case "$_ver" in
  *rke2*) echo "    Distro: RKE2 ($_ver)" ;;
  *k3s*)  echo "    Distro: K3s ($_ver)" ;;
  *)      echo "    Distro: Kubernetes ($_ver)" ;;
esac
`, ctx))
	}
	return b.String()
}

func (a *App) clusterStatsScript(ctxs []string) string {
	var b strings.Builder
	for _, ctx := range ctxs {
		b.WriteString(fmt.Sprintf(`
_ctx="%s"
echo "  [$_ctx]"
_nodes=$(kubectl --context "$_ctx" get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')
_pods=$(kubectl --context "$_ctx" get pods -A --no-headers 2>/dev/null | wc -l | tr -d ' ')
_ns=$(kubectl --context "$_ctx" get ns --no-headers 2>/dev/null | wc -l | tr -d ' ')
_deploy=$(kubectl --context "$_ctx" get deploy -A --no-headers 2>/dev/null | wc -l | tr -d ' ')
printf '    Nodes: %%s  Pods: %%s  Namespaces: %%s  Deployments: %%s\n' "$_nodes" "$_pods" "$_ns" "$_deploy"
kubectl --context "$_ctx" top nodes --no-headers 2>/dev/null | awk '{cpu+=$2; mem+=$4; n++} END {if(n>0) printf "    CPU: %%s  MEM: %%s (%%d nodes)\n", cpu/n"%%", mem/n"%%", n}' || echo '    (metrics unavailable)'
`, ctx))
	}
	return b.String()
}

func (a *App) ecosystemDetectScript(ctxs []string) string {
	var b strings.Builder
	for _, ctx := range ctxs {
		b.WriteString(fmt.Sprintf(`
_ctx="%s"
echo "  [$_ctx]"
for _comp in "longhorn-system:Longhorn" "cattle-system:Rancher" "cattle-fleet-system:Fleet" "kubevirt:KubeVirt" "harvester-system:Harvester" "kubewarden:Kubewarden" "gpu-operator:GPU Operator" "cattle-monitoring-system:Monitoring"; do
  _ns="${_comp%%:*}"
  _name="${_comp#*:}"
  if kubectl --context "$_ctx" get ns "$_ns" >/dev/null 2>&1; then
    printf '    ✓ %%s\n' "$_name"
  else
    printf '    ✗ %%s\n' "$_name"
  fi
done
`, ctx))
	}
	return b.String()
}

func (a *App) rk9sRke2K3sDashboard() {
	ctxs, subject := a.dashContexts()
	ctxList := ctxListArg(ctxs)
	script := fmt.Sprintf(`
echo '╔══════════════════════════════════════════════════════╗'
echo '║           RKE2 / K3s Configuration                   ║'
echo '╚══════════════════════════════════════════════════════╝'
echo ''
echo 'Contexts: %s'
echo ''

echo '=== Distribution Detection ==='
for _ctx in %s; do
  echo "  [$_ctx]"
  _ver=$(kubectl --context "$_ctx" get nodes -o jsonpath='{.items[0].status.nodeInfo.kubeletVersion}' 2>/dev/null)
  case "$_ver" in
    *rke2*) _distro="RKE2" ;;
    *k3s*)  _distro="K3s" ;;
    *)      _distro="Unknown" ;;
  esac
  _nodes=$(kubectl --context "$_ctx" get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')
  _cp=$(kubectl --context "$_ctx" get nodes -l node-role.kubernetes.io/control-plane= --no-headers 2>/dev/null | wc -l | tr -d ' ')
  _workers=$(kubectl --context "$_ctx" get nodes -l '!node-role.kubernetes.io/control-plane' --no-headers 2>/dev/null | wc -l | tr -d ' ')
  echo "    Distro: $_distro  Version: $_ver"
  echo "    Nodes: $_nodes (control-plane: $_cp, workers: $_workers)"
done
echo ''

echo '=== Node Configuration (config.yaml from control-plane) ==='
for _ctx in %s; do
  echo "  [$_ctx]"
  _node=$(kubectl --context "$_ctx" get nodes -l node-role.kubernetes.io/control-plane= -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  if [ -n "$_node" ]; then
    echo "    node: $_node"
    kubectl --context "$_ctx" debug "node/$_node" -it --image=alpine:3.18 -- chroot /host sh -c '
      for cfg in /etc/rancher/rke2/config.yaml /etc/rancher/k3s/config.yaml; do
        if [ -f "$cfg" ]; then
          echo "    --- $cfg ---"
          cat "$cfg" 2>/dev/null | sed "s/^/    /"
          echo ""
          break
        fi
      done
      if [ ! -f /etc/rancher/rke2/config.yaml ] && [ ! -f /etc/rancher/k3s/config.yaml ]; then
        echo "    (no RKE2/K3s config.yaml found)"
      fi
    ' 2>/dev/null || echo '    (kubectl debug not available or node not accessible)'
  else
    echo "    (no control-plane node found)"
  fi
done
echo ''

echo '=== Installed Components (HelmCharts) ==='
%s
echo ''

echo '=== System Pods (kube-system) ==='
%s
echo ''

echo '=== config.yaml Reference ==='
echo '  ┌──────────────────────────────────────────────────┐'
echo '  │ Cluster Networking & API                          │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ write-kubeconfig-mode  Permissions (def: 0600)   │'
echo '  │ tls-san                Extra SANs for API cert   │'
echo '  │ bind-address           Server bind IP (0.0.0.0)  │'
echo '  │ https-listen-port      API port (def: 6443)      │'
echo '  │ cluster-cidr           Pod CIDR (10.42.0.0/16)   │'
echo '  │ service-cidr           Svc CIDR (10.43.0.0/16)   │'
echo '  │ cluster-dns            DNS IP (10.43.0.10)       │'
echo '  │ cluster-domain         Domain (cluster.local)    │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ Authentication & HA                               │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ token                  Shared join secret         │'
echo '  │ cluster-init           Init embedded etcd (bool)  │'
echo '  │ datastore-endpoint     External DB URL            │'
echo '  │ server                 Join URL (agents/2nd srv)  │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ Component Args (pass-through lists)               │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ kube-apiserver-arg     API server args            │'
echo '  │ kube-scheduler-arg     Scheduler args             │'
echo '  │ kube-controller-manager-arg  Controller mgr args  │'
echo '  │ kubelet-arg            Kubelet args               │'
echo '  │ kube-proxy-arg         Kube-proxy args            │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ Bundled Components                                │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ disable                List to not deploy:        │'
echo '  │   K3s:  traefik servicelb coredns local-storage   │'
echo '  │         metrics-server                            │'
echo '  │   RKE2: rke2-canal rke2-coredns                  │'
echo '  │         rke2-ingress-nginx rke2-metrics-server    │'
echo '  │ cni                    CNI (canal/calico/cilium)  │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ Node Configuration                                │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ node-name              Override hostname           │'
echo '  │ node-label             Labels (tier=frontend)     │'
echo '  │ node-taint             Taints (key=val:NoSched)   │'
echo '  │ selinux                SELinux support (bool)     │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ Storage & Etcd Backup                             │'
echo '  ├──────────────────────────────────────────────────┤'
echo '  │ etcd-snapshot-schedule-cron  Backup cron          │'
echo '  │ etcd-snapshot-retention      Keep N snaps (5)     │'
echo '  │ etcd-s3                      S3 backup (bool)     │'
echo '  └──────────────────────────────────────────────────┘'
echo ''
echo '  To view full options: rke2 server --help / k3s server --help'
echo '  To edit: kubectl debug node/<name> -it --image=alpine:3.18'
echo '           then: vi /host/etc/rancher/rke2/config.yaml'
`,
		subject,
		ctxList,
		ctxList,
		mcKubectl(ctxs, "get helmcharts.helm.cattle.io -n kube-system -o custom-columns='NAME:.metadata.name,CHART:.spec.chart,VERSION:.spec.version,NS:.spec.targetNamespace' 2>/dev/null || echo '  (no HelmChart CRDs)'"),
		mcKubectl(ctxs, "-n kube-system get pods -o custom-columns='NAME:.metadata.name,STATUS:.status.phase,NODE:.spec.nodeName,RESTARTS:.status.containerStatuses[0].restartCount' 2>/dev/null | head -25"),
	)
	a.runDashScript("RKE2/K3s", subject, script)
}

func (a *App) rk9sDashboard(name string) {
	switch name {
	case "home":
		a.rk9sHomeDashboard()
		return
	case "rke2k3s":
		a.rk9sRke2K3sDashboard()
		return
	case "k3k":
		a.rk9sK3kDashboard()
		return
	case "kubewarden":
		a.rk9sKubewardenDashboard()
		return
	case "releases":
		a.rk9sReleasesDashboard()
		return
	case "etcd":
		ctxs, subject := a.dashContexts()
		a.runDashScript("etcd", subject, fmt.Sprintf(`
echo '=== etcd Dashboard ==='
echo 'Contexts: %s'
echo ''
echo '--- etcd Pods ---'
%s
echo ''
echo '--- etcd Health (via kubectl exec) ---'
for _ctx in %s; do
  echo "  [$_ctx]"
  _pod=$(kubectl --context "$_ctx" -n kube-system get pods -l component=etcd -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  if [ -z "$_pod" ]; then
    _pod=$(kubectl --context "$_ctx" -n kube-system get pods -l tier=control-plane -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | grep etcd | head -1)
  fi
  if [ -n "$_pod" ]; then
    kubectl --context "$_ctx" -n kube-system exec "$_pod" -- sh -c 'ETCDCTL_API=3 etcdctl --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/server.crt --key /etc/kubernetes/pki/etcd/server.key endpoint health 2>&1' 2>/dev/null || echo '    (exec failed – try kubectl debug node)'
  else
    echo '    (no etcd pod found – may be external etcd)'
  fi
done
echo ''
echo '--- etcd Member List ---'
for _ctx in %s; do
  echo "  [$_ctx]"
  _pod=$(kubectl --context "$_ctx" -n kube-system get pods -l component=etcd -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  if [ -z "$_pod" ]; then
    _pod=$(kubectl --context "$_ctx" -n kube-system get pods -l tier=control-plane -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | grep etcd | head -1)
  fi
  if [ -n "$_pod" ]; then
    kubectl --context "$_ctx" -n kube-system exec "$_pod" -- sh -c 'ETCDCTL_API=3 etcdctl --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/server.crt --key /etc/kubernetes/pki/etcd/server.key member list -w table 2>&1' 2>/dev/null || echo '    (member list failed)'
  fi
done
echo ''
echo '--- etcd DB Size & Alarms ---'
for _ctx in %s; do
  echo "  [$_ctx]"
  _pod=$(kubectl --context "$_ctx" -n kube-system get pods -l component=etcd -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  if [ -z "$_pod" ]; then
    _pod=$(kubectl --context "$_ctx" -n kube-system get pods -l tier=control-plane -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | grep etcd | head -1)
  fi
  if [ -n "$_pod" ]; then
    kubectl --context "$_ctx" -n kube-system exec "$_pod" -- sh -c 'ETCDCTL_API=3 etcdctl --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/server.crt --key /etc/kubernetes/pki/etcd/server.key endpoint status -w table 2>&1' 2>/dev/null || echo '    (status failed)'
    echo "    Alarms:"
    kubectl --context "$_ctx" -n kube-system exec "$_pod" -- sh -c 'ETCDCTL_API=3 etcdctl --cacert /etc/kubernetes/pki/etcd/ca.crt --cert /etc/kubernetes/pki/etcd/server.crt --key /etc/kubernetes/pki/etcd/server.key alarm list 2>&1' 2>/dev/null || echo '    (alarm list failed)'
  fi
done
echo ''
echo '--- RKE2/K3s etcd (via kubectl debug) ---'
for _ctx in %s; do
  echo "  [$_ctx]"
  _node=$(kubectl --context "$_ctx" get nodes -l node-role.kubernetes.io/control-plane= -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  if [ -n "$_node" ]; then
    echo "    node: $_node"
    kubectl --context "$_ctx" debug "node/$_node" -it --image=alpine/k8s:1.31 -- sh -c '
      crt=$(ls /host/var/lib/rancher/*/server/tls/etcd/server-client.crt 2>/dev/null | head -1)
      key=$(ls /host/var/lib/rancher/*/server/tls/etcd/server-client.key 2>/dev/null | head -1)
      ca=$(ls /host/var/lib/rancher/*/server/tls/etcd/server-ca.crt 2>/dev/null | head -1)
      if [ -n "$crt" ]; then
        ETCDCTL_API=3 etcdctl --cacert "$ca" --cert "$crt" --key "$key" --endpoints https://127.0.0.1:2379 endpoint health 2>&1
        echo "---"
        ETCDCTL_API=3 etcdctl --cacert "$ca" --cert "$crt" --key "$key" --endpoints https://127.0.0.1:2379 endpoint status -w table 2>&1
        echo "---"
        ETCDCTL_API=3 etcdctl --cacert "$ca" --cert "$crt" --key "$key" --endpoints https://127.0.0.1:2379 alarm list 2>&1
      else
        echo "    (RKE2/K3s etcd certs not found)"
      fi
    ' 2>/dev/null || echo '    (kubectl debug failed or not RKE2/K3s)'
  fi
done
echo ''
echo 'Plugin shortcuts (in nodes view):'
echo '  Shift-E  etcdctl health       Shift-N  etcd snapshot'
echo '  Shift-F  etcd defrag           Shift-A  etcd alarm disarm'
`,
			subject,
			mcKubectl(ctxs, "-n kube-system get pods -l component=etcd -o wide 2>/dev/null || echo '  (no etcd pods – may use embedded or external etcd)'"),
			ctxListArg(ctxs),
			ctxListArg(ctxs),
			ctxListArg(ctxs),
			ctxListArg(ctxs),
		))
	}
}

func (a *App) rk9sK3kDashboard() {
	ctxs, subject := a.dashContexts()
	a.runDashScript("K3k", subject, fmt.Sprintf(`
echo '=== K3k Dashboard (Kubernetes-in-Kubernetes) ==='
echo 'Contexts: %s'
echo ''
echo '--- K3k Clusters ---'
%s
echo ''
echo '--- K3k Cluster Status ---'
for _ctx in %s; do
  echo "  [$_ctx]"
  kubectl --context "$_ctx" get clusters.k3k.io -A -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,VERSION:.spec.version,SERVERS:.spec.servers,AGENTS:.spec.agents,READY:.status.ready' 2>/dev/null || echo '    (no K3k clusters or CRD not installed)'
done
echo ''
echo '--- K3k Pods ---'
%s
echo ''
echo '--- K3k Server Nodes (per cluster) ---'
for _ctx in %s; do
  echo "  [$_ctx]"
  for _cluster in $(kubectl --context "$_ctx" get clusters.k3k.io -A -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}' 2>/dev/null); do
    _ns=$(echo "$_cluster" | cut -d/ -f1)
    _name=$(echo "$_cluster" | cut -d/ -f2)
    echo "    cluster: $_name (ns: $_ns)"
    kubectl --context "$_ctx" -n "$_ns" get pods -l "cluster.k3k.io/cluster-name=$_name,cluster.k3k.io/role=server" -o wide 2>/dev/null | head -10 || echo '      (no server pods)'
  done
done
echo ''
echo 'Commands:'
echo '  :clusters.k3k.io    K3k virtual clusters'
echo '  :pods k3k-system    K3k controller pods'
`,
		subject,
		mcKubectl(ctxs, "get clusters.k3k.io -A 2>/dev/null || echo '  (K3k CRD not installed)'"),
		ctxListArg(ctxs),
		mcKubectl(ctxs, "-n k3k-system get pods 2>/dev/null || echo '  (k3k-system namespace not found)'"),
		ctxListArg(ctxs),
	))
}

func (a *App) rk9sKubewardenDashboard() {
	ctxs, subject := a.dashContexts()
	a.runDashScript("Kubewarden", subject, fmt.Sprintf(`
echo '=== Kubewarden Dashboard ==='
echo 'Contexts: %s'
echo ''
echo '--- Policy Servers ---'
%s
echo ''
echo '--- Cluster Admission Policies ---'
%s
echo ''
echo '--- Admission Policies (namespaced) ---'
%s
echo ''
echo '--- Policy Server Status ---'
for _ctx in %s; do
  echo "  [$_ctx]"
  kubectl --context "$_ctx" get policyservers.policies.kubewarden.io -o custom-columns='NAME:.metadata.name,REPLICAS:.spec.replicas,SERVICE:.status.service,READY:.status.conditions[?(@.type=="PolicyServerSecretReconciled")].status' 2>/dev/null || echo '    (no policy servers)'
done
echo ''
echo '--- Controller Pods ---'
%s
echo ''
echo '--- Policy Reports ---'
%s
echo ''
echo 'Commands:'
echo '  :clusteradmissionpolicies.policies.kubewarden.io   Cluster policies'
echo '  :admissionpolicies.policies.kubewarden.io          Namespaced policies'
echo '  :policyservers.policies.kubewarden.io              Policy servers'
`,
		subject,
		mcKubectl(ctxs, "get policyservers.policies.kubewarden.io 2>/dev/null || echo '  (Kubewarden not installed)'"),
		mcKubectl(ctxs, "get clusteradmissionpolicies.policies.kubewarden.io -o custom-columns='NAME:.metadata.name,POLICY:.spec.module,MODE:.spec.mode,STATUS:.status.policyStatus' 2>/dev/null || echo '  (none)'"),
		mcKubectl(ctxs, "get admissionpolicies.policies.kubewarden.io -A -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,POLICY:.spec.module,MODE:.spec.mode' 2>/dev/null || echo '  (none)'"),
		ctxListArg(ctxs),
		mcKubectl(ctxs, "-n kubewarden get pods 2>/dev/null || kubectl -n cattle-kubewarden-system get pods 2>/dev/null || echo '  (controller pods not found)'"),
		mcKubectl(ctxs, "get policyreports.wgpolicyk8s.io -A --no-headers 2>/dev/null | head -10 || echo '  (no policy reports)'"),
	))
}

func (a *App) rk9sReleasesDashboard() {
	a.runDashScript("Releases", "SUSE Rancher Ecosystem", `
echo '=== SUSE Rancher Ecosystem Releases ==='
echo ''
echo '--- Rancher ---'
curl -sL "https://api.github.com/repos/rancher/rancher/releases?per_page=5" 2>/dev/null | \
  jq -r '.[] | "  \(.tag_name)  \(.published_at | split("T")[0])  \(.prerelease | if . then "(pre)" else "" end)"' 2>/dev/null || \
  echo '  (failed to fetch – check network)'
echo ''
echo '--- Harvester ---'
curl -sL "https://api.github.com/repos/harvester/harvester/releases?per_page=5" 2>/dev/null | \
  jq -r '.[] | "  \(.tag_name)  \(.published_at | split("T")[0])  \(.prerelease | if . then "(pre)" else "" end)"' 2>/dev/null || \
  echo '  (failed to fetch)'
echo ''
echo '--- Longhorn ---'
curl -sL "https://api.github.com/repos/longhorn/longhorn/releases?per_page=5" 2>/dev/null | \
  jq -r '.[] | "  \(.tag_name)  \(.published_at | split("T")[0])  \(.prerelease | if . then "(pre)" else "" end)"' 2>/dev/null || \
  echo '  (failed to fetch)'
echo ''
echo '--- RKE2 ---'
curl -sL "https://api.github.com/repos/rancher/rke2/releases?per_page=5" 2>/dev/null | \
  jq -r '.[] | "  \(.tag_name)  \(.published_at | split("T")[0])  \(.prerelease | if . then "(pre)" else "" end)"' 2>/dev/null || \
  echo '  (failed to fetch)'
echo ''
echo '--- K3s ---'
curl -sL "https://api.github.com/repos/k3s-io/k3s/releases?per_page=5" 2>/dev/null | \
  jq -r '.[] | "  \(.tag_name)  \(.published_at | split("T")[0])  \(.prerelease | if . then "(pre)" else "" end)"' 2>/dev/null || \
  echo '  (failed to fetch)'
echo ''
echo '--- Fleet ---'
curl -sL "https://api.github.com/repos/rancher/fleet/releases?per_page=5" 2>/dev/null | \
  jq -r '.[] | "  \(.tag_name)  \(.published_at | split("T")[0])  \(.prerelease | if . then "(pre)" else "" end)"' 2>/dev/null || \
  echo '  (failed to fetch)'
echo ''
echo '--- Kubewarden ---'
curl -sL "https://api.github.com/repos/kubewarden/kubewarden-controller/releases?per_page=5" 2>/dev/null | \
  jq -r '.[] | "  \(.tag_name)  \(.published_at | split("T")[0])  \(.prerelease | if . then "(pre)" else "" end)"' 2>/dev/null || \
  echo '  (failed to fetch)'
echo ''
echo '--- K3k ---'
curl -sL "https://api.github.com/repos/rancher/k3k/releases?per_page=5" 2>/dev/null | \
  jq -r '.[] | "  \(.tag_name)  \(.published_at | split("T")[0])  \(.prerelease | if . then "(pre)" else "" end)"' 2>/dev/null || \
  echo '  (failed to fetch)'
echo ''
echo 'Links:'
echo '  https://github.com/rancher/rancher/releases'
echo '  https://github.com/harvester/harvester/releases'
echo '  https://github.com/longhorn/longhorn/releases'
`)
}

func (a *App) dirCmd(path string, pushCmd bool) error {
	slog.Debug("Exec Dir command", slogs.Path, path)
	_, err := os.Stat(path)
	if err != nil {
		return err
	}
	if path == "." {
		dir, err := os.Getwd()
		if err == nil {
			path = dir
		}
	}
	if pushCmd {
		a.cmdHistory.Push("dir " + path)
	}

	return a.inject(NewDir(path), true)
}

func (a *App) quitCmd(evt *tcell.EventKey) *tcell.EventKey {
	noExit := a.Config.K9s.NoExitOnCtrlC
	if a.InCmdMode() {
		if isBailoutEvt(evt) && noExit {
			return nil
		}
		return evt
	}

	if !noExit {
		a.BailOut(0)
	}

	return nil
}

func (a *App) helpCmd(evt *tcell.EventKey) *tcell.EventKey {
	if evt != nil && evt.Rune() == '?' && a.Prompt().InCmdMode() {
		return evt
	}

	top := a.Content.Top()
	if top != nil && top.Name() == "help" {
		a.Content.Pop()
		return nil
	}

	if err := a.inject(NewHelp(a), false); err != nil {
		a.Flash().Err(err)
	}

	a.Prompt().Deactivate()
	return nil
}

// previousCommand returns to the command prior to the current one in the history
func (a *App) previousCommand(evt *tcell.EventKey) *tcell.EventKey {
	if evt != nil && evt.Rune() == rune(ui.KeyLeftBracket) && a.Prompt().InCmdMode() {
		return evt
	}
	c, ok := a.cmdHistory.Back()
	if !ok {
		a.App.Flash().Warn("Can't go back any further")
		return evt
	}
	a.gotoResource(c, "", true, false)
	return nil
}

// nextCommand returns to the command subsequent to the current one in the history
func (a *App) nextCommand(evt *tcell.EventKey) *tcell.EventKey {
	if evt != nil && evt.Rune() == rune(ui.KeyRightBracket) && a.Prompt().InCmdMode() {
		return evt
	}
	c, ok := a.cmdHistory.Forward()
	if !ok {
		a.App.Flash().Warn("Can't go forward any further")
		return evt
	}
	// We go to the resource before updating the history so that
	// gotoResource doesn't add this command to the history
	a.gotoResource(c, "", true, false)
	return nil
}

// lastCommand switches between the last command and the current one a la `cd -`
func (a *App) lastCommand(evt *tcell.EventKey) *tcell.EventKey {
	if evt != nil && evt.Rune() == ui.KeyDash && a.Prompt().InCmdMode() {
		return evt
	}
	c, ok := a.cmdHistory.Top()
	if !ok {
		a.App.Flash().Warn("No previous view to switch to")
		return evt
	}
	a.gotoResource(c, "", true, false)

	return nil
}

func (a *App) aliasCmd(*tcell.EventKey) *tcell.EventKey {
	if a.Content.Top() != nil && a.Content.Top().Name() == aliasTitle {
		a.Content.Pop()
		return nil
	}

	if err := a.inject(NewAlias(client.AliGVR), false); err != nil {
		a.Flash().Err(err)
	}

	return nil
}

func (a *App) gotoResource(c, path string, clearStack, pushCmd bool) {
	err := a.command.run(cmd.NewInterpreter(c), path, clearStack, pushCmd)
	if err != nil {
		d := a.Styles.Dialog()
		dialog.ShowError(&d, a.Content.Pages, err.Error())
	}
}

func (a *App) inject(c model.Component, clearStack bool) error {
	ctx := context.WithValue(context.Background(), internal.KeyApp, a)
	if err := c.Init(ctx); err != nil {
		slog.Error("Component init failed",
			slogs.Error, err,
			slogs.CompName, c.Name(),
		)
		return err
	}
	if clearStack {
		a.Content.Clear()
	}
	a.Content.Push(c)

	return nil
}

func (a *App) clusterInfo() *ClusterInfo {
	return a.Views()["clusterInfo"].(*ClusterInfo)
}

func (a *App) statusIndicator() *ui.StatusIndicator {
	return a.Views()["statusIndicator"].(*ui.StatusIndicator)
}
