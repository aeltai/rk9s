// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package config

import (
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/derailed/k9s/internal/config/data"
	"github.com/derailed/k9s/internal/rk9s"
	"github.com/derailed/k9s/internal/slogs"
	"gopkg.in/yaml.v3"
)

//go:embed default_plugins/*.yaml
var defaultPluginsFS embed.FS

const (
	// K9sEnvConfigDir represents k9s configuration dir env var.
	K9sEnvConfigDir = "K9S_CONFIG_DIR"

	// K9sEnvLogsDir represents k9s logs dir env var.
	K9sEnvLogsDir = "K9S_LOGS_DIR"

	// AppName tracks rk9s app name.
	AppName = "rk9s"

	K9sLogsFile = "rk9s.log"
)

var (
	//go:embed templates/benchmarks.yaml
	// benchmarkTpl tracks benchmark default config template
	benchmarkTpl []byte

	//go:embed templates/aliases.yaml
	// aliasesTpl tracks aliases default config template
	aliasesTpl []byte

	//go:embed templates/hotkeys.yaml
	// hotkeysTpl tracks hotkeys default config template
	hotkeysTpl []byte

	//go:embed templates/stock-skin.yaml
	// stockSkinTpl tracks stock skin template
	stockSkinTpl []byte
)

var (
	// AppConfigDir tracks main k9s config home directory.
	AppConfigDir string

	// AppSkinsDir tracks skins data directory.
	AppSkinsDir string

	// AppBenchmarksDir tracks benchmarks results directory.
	AppBenchmarksDir string

	// AppDumpsDir tracks screen dumps data directory.
	AppDumpsDir string

	// AppContextsDir tracks contexts data directory.
	AppContextsDir string

	// AppConfigFile tracks k9s config file.
	AppConfigFile string

	// AppLogFile tracks k9s logs file.
	AppLogFile string

	// AppViewsFile tracks custom views config file.
	AppViewsFile string

	// AppAliasesFile tracks aliases config file.
	AppAliasesFile string

	// AppPluginsFile tracks plugins config file.
	AppPluginsFile string

	// AppHotKeysFile tracks hotkeys config file.
	AppHotKeysFile string
)

// InitLogLoc initializes K9s logs location.
func InitLogLoc() error {
	var appLogDir string
	switch {
	case isEnvSet(K9sEnvLogsDir):
		appLogDir = os.Getenv(K9sEnvLogsDir)
	case isEnvSet(K9sEnvConfigDir):
		tmpDir, err := UserTmpDir()
		if err != nil {
			return err
		}
		appLogDir = tmpDir
	default:
		var err error
		appLogDir, err = xdg.StateFile(AppName)
		if err != nil {
			return err
		}
	}
	if err := data.EnsureFullPath(appLogDir, data.DefaultDirMod); err != nil {
		return err
	}
	AppLogFile = filepath.Join(appLogDir, K9sLogsFile)

	return nil
}

// InitLocs initializes k9s artifacts locations.
func InitLocs() error {
	var err error
	if isEnvSet(K9sEnvConfigDir) {
		err = initK9sEnvLocs()
	} else {
		err = initXDGLocs()
	}
	if err != nil {
		return err
	}
	_ = EnsureDefaultPlugins() // copy embedded plugins on first run
	_ = EnsureDefaultHotkeys() // sync rk9s navigation hotkeys
	return nil
}

func initK9sEnvLocs() error {
	AppConfigDir = os.Getenv(K9sEnvConfigDir)
	if err := data.EnsureFullPath(AppConfigDir, data.DefaultDirMod); err != nil {
		return err
	}

	AppDumpsDir = filepath.Join(AppConfigDir, "screen-dumps")
	if err := data.EnsureFullPath(AppDumpsDir, data.DefaultDirMod); err != nil {
		slog.Warn("Unable to create screen-dumps dir", slogs.Dir, AppDumpsDir, slogs.Error, err)
	}
	AppBenchmarksDir = filepath.Join(AppConfigDir, "benchmarks")
	if err := data.EnsureFullPath(AppBenchmarksDir, data.DefaultDirMod); err != nil {
		slog.Warn("Unable to create benchmarks dir",
			slogs.Dir, AppBenchmarksDir,
			slogs.Error, err,
		)
	}
	AppSkinsDir = filepath.Join(AppConfigDir, "skins")
	if err := data.EnsureFullPath(AppSkinsDir, data.DefaultDirMod); err != nil {
		slog.Warn("Unable to create skins dir",
			slogs.Dir, AppSkinsDir,
			slogs.Error, err,
		)
	}
	AppContextsDir = filepath.Join(AppConfigDir, "clusters")
	if err := data.EnsureFullPath(AppContextsDir, data.DefaultDirMod); err != nil {
		slog.Warn("Unable to create clusters dir",
			slogs.Dir, AppContextsDir,
			slogs.Error, err,
		)
	}

	AppConfigFile = filepath.Join(AppConfigDir, data.MainConfigFile)
	AppHotKeysFile = filepath.Join(AppConfigDir, "hotkeys.yaml")
	AppAliasesFile = filepath.Join(AppConfigDir, "aliases.yaml")
	AppPluginsFile = filepath.Join(AppConfigDir, "plugins.yaml")
	AppViewsFile = filepath.Join(AppConfigDir, "views.yaml")

	return nil
}

func initXDGLocs() error {
	var err error

	AppConfigDir, err = xdg.ConfigFile(AppName)
	if err != nil {
		return err
	}

	AppConfigFile, err = xdg.ConfigFile(filepath.Join(AppName, data.MainConfigFile))
	if err != nil {
		return err
	}

	AppHotKeysFile = filepath.Join(AppConfigDir, "hotkeys.yaml")
	AppAliasesFile = filepath.Join(AppConfigDir, "aliases.yaml")
	AppPluginsFile = filepath.Join(AppConfigDir, "plugins.yaml")
	AppViewsFile = filepath.Join(AppConfigDir, "views.yaml")

	AppSkinsDir = filepath.Join(AppConfigDir, "skins")
	if e := data.EnsureFullPath(AppSkinsDir, data.DefaultDirMod); e != nil {
		slog.Warn("No skins dir detected", slogs.Error, e)
	}

	AppDumpsDir, err = xdg.StateFile(filepath.Join(AppName, "screen-dumps"))
	if err != nil {
		return err
	}

	AppBenchmarksDir, err = xdg.StateFile(filepath.Join(AppName, "benchmarks"))
	if err != nil {
		slog.Warn("No benchmarks dir detected",
			slogs.Dir, AppBenchmarksDir,
			slogs.Error, err,
		)
	}

	dataDir, err := xdg.DataFile(AppName)
	if err != nil {
		return err
	}
	AppContextsDir = filepath.Join(dataDir, "clusters")
	if err := data.EnsureFullPath(AppContextsDir, data.DefaultDirMod); err != nil {
		slog.Warn("No context dir detected",
			slogs.Dir, AppContextsDir,
			slogs.Error, err,
		)
	}

	return nil
}

// AppContextDir generates a valid context config dir.
func AppContextDir(cluster, context string) string {
	return filepath.Join(AppContextsDir, data.SanitizeContextSubpath(cluster, context))
}

// AppContextAliasesFile generates a valid context specific aliases file path.
func AppContextAliasesFile(cluster, context string) string {
	return filepath.Join(AppContextsDir, data.SanitizeContextSubpath(cluster, context), "aliases.yaml")
}

// AppContextPluginsFile generates a valid context specific plugins file path.
func AppContextPluginsFile(cluster, context string) string {
	return filepath.Join(AppContextsDir, data.SanitizeContextSubpath(cluster, context), "plugins.yaml")
}

// AppContextHotkeysFile generates a valid context specific hotkeys file path.
func AppContextHotkeysFile(cluster, context string) string {
	return filepath.Join(AppContextsDir, data.SanitizeContextSubpath(cluster, context), "hotkeys.yaml")
}

// AppContextConfig generates a valid context config file path.
func AppContextConfig(cluster, context string) string {
	return filepath.Join(AppContextDir(cluster, context), data.MainConfigFile)
}

// DumpsDir generates a valid context dump directory.
func DumpsDir(cluster, context string) (string, error) {
	dir := filepath.Join(AppDumpsDir, data.SanitizeContextSubpath(cluster, context))

	return dir, data.EnsureDirPath(dir, data.DefaultDirMod)
}

// EnsureBenchmarksDir generates a valid benchmark results directory.
func EnsureBenchmarksDir(cluster, context string) (string, error) {
	dir := filepath.Join(AppBenchmarksDir, data.SanitizeContextSubpath(cluster, context))

	return dir, data.EnsureDirPath(dir, data.DefaultDirMod)
}

// EnsureBenchmarksCfgFile generates a valid benchmark file.
func EnsureBenchmarksCfgFile(cluster, context string) (string, error) {
	f := filepath.Join(AppContextDir(cluster, context), "benchmarks.yaml")
	if err := data.EnsureDirPath(f, data.DefaultDirMod); err != nil {
		return "", err
	}
	if _, err := os.Stat(f); errors.Is(err, fs.ErrNotExist) {
		return f, os.WriteFile(f, benchmarkTpl, data.DefaultFileMod)
	}

	return f, nil
}

// EnsureAliasesCfgFile generates a valid aliases file.
func EnsureAliasesCfgFile() (string, error) {
	f := filepath.Join(AppConfigDir, "aliases.yaml")
	if err := data.EnsureDirPath(f, data.DefaultDirMod); err != nil {
		return "", err
	}
	if _, err := os.Stat(f); errors.Is(err, fs.ErrNotExist) {
		return f, os.WriteFile(f, aliasesTpl, data.DefaultFileMod)
	}

	return f, nil
}

// EnsureHotkeysCfgFile generates a valid hotkeys file.
func EnsureHotkeysCfgFile() (string, error) {
	f := filepath.Join(AppConfigDir, "hotkeys.yaml")
	if err := data.EnsureDirPath(f, data.DefaultDirMod); err != nil {
		return "", err
	}
	if _, err := os.Stat(f); errors.Is(err, fs.ErrNotExist) {
		return f, os.WriteFile(f, hotkeysTpl, data.DefaultFileMod)
	}

	return f, nil
}

// SkinFileFromName generate skin file path from spec.
func SkinFileFromName(n string) string {
	if n == "" {
		n = "stock"
	}

	return filepath.Join(AppSkinsDir, n+".yaml")
}

// SelectedContextsPath returns the path for rk9s multi-context selection.
func SelectedContextsPath() string {
	path, err := xdg.ConfigFile(filepath.Join(AppName, "selected_contexts"))
	if err != nil {
		return filepath.Join(AppConfigDir, "selected_contexts")
	}
	return path
}

// LoadSelectedContexts reads the list of selected contexts (one per line).
func LoadSelectedContexts() ([]string, error) {
	path := SelectedContextsPath()
	bb, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(bb)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

// SaveSelectedContexts writes the selected contexts to disk.
func SaveSelectedContexts(ctxs []string) error {
	path := SelectedContextsPath()
	if err := data.EnsureDirPath(filepath.Dir(path), data.DefaultDirMod); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(ctxs, "\n")), 0600)
}

// EnsureDefaultHotkeys writes rk9s dashboard hotkeys to the hotkeys config file.
// Merges with existing user hotkeys; always overwrites rk9s-* entries so dashboards stay correct.
func EnsureDefaultHotkeys() error {
	if AppHotKeysFile == "" {
		return nil
	}
	rk9sHotkeys := make(map[string]HotKey, len(rk9s.DefaultHotkeys())+3)
	for _, hk := range rk9s.DefaultHotkeys() {
		rk9sHotkeys[hk.ID] = HotKey{
			ShortCut:    hk.ShortCut,
			Override:    true,
			Description: hk.Description,
			Command:     hk.Command,
		}
	}

	existing := NewHotKeys()
	_ = existing.LoadHotKeys(AppHotKeysFile)

	oldKeys := []string{"rk9s-rke2k3s"}
	for _, k := range oldKeys {
		if _, ok := existing.HotKey[k]; ok {
			delete(existing.HotKey, k)
		}
	}

	changed := false
	for k, v := range rk9sHotkeys {
		prev, ok := existing.HotKey[k]
		if !ok || prev.Command != v.Command || prev.ShortCut != v.ShortCut || prev.Override != v.Override {
			existing.HotKey[k] = v
			changed = true
		}
	}
	if !changed {
		return nil
	}

	bb, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}
	slog.Info("Synced rk9s hotkeys", slogs.Path, AppHotKeysFile)
	return os.WriteFile(AppHotKeysFile, bb, 0644)
}

// EnsureDefaultPlugins syncs embedded rk9s plugins to the user's plugins dir.
// Always writes new or updated plugins so upgrades deploy automatically.
func EnsureDefaultPlugins() error {
	dir, err := xdg.DataFile(filepath.Join(AppName, "plugins"))
	if err != nil {
		return nil
	}
	entries, _ := fs.ReadDir(defaultPluginsFS, "default_plugins")
	if len(entries) == 0 {
		return nil
	}
	if err := data.EnsureFullPath(dir, data.DefaultDirMod); err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		embedded, err := fs.ReadFile(defaultPluginsFS, filepath.Join("default_plugins", e.Name()))
		if err != nil {
			slog.Warn("Could not read embedded plugin", slogs.Path, e.Name(), slogs.Error, err)
			continue
		}
		dst := filepath.Join(dir, e.Name())
		existing, readErr := os.ReadFile(dst)
		if readErr == nil && string(existing) == string(embedded) {
			continue
		}
		if err := os.WriteFile(dst, embedded, 0644); err != nil {
			slog.Warn("Could not write default plugin", slogs.Path, dst, slogs.Error, err)
		} else {
			slog.Info("Synced rk9s plugin", slogs.Path, e.Name())
		}
	}
	return nil
}
