package accounts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/LFroesch/sb/internal/config"
)

type Slot struct {
	Provider string
	Name     string
	SavedAt  time.Time
	Active   bool
}

type Status struct {
	Active map[string]string
}

type registry struct {
	Active map[string]string `json:"active,omitempty"`
}

type providerSpec struct {
	name           string
	liveDir        func() string
	required       string
	files          []string
	slotHomeEnvVar string
	snapshotWhole  bool
}

type authIdentity struct {
	AccountID string
}

func SaveCurrent(provider, name string) error {
	spec, err := resolveProvider(provider)
	if err != nil {
		return err
	}
	if err := validateName(name); err != nil {
		return err
	}
	if err := ensureRoot(); err != nil {
		return err
	}
	liveDir := spec.liveDir()
	if strings.TrimSpace(liveDir) == "" {
		return fmt.Errorf("%s live config dir is empty", spec.name)
	}
	if spec.snapshotWhole {
		if err := syncSlotFromLiveHome(spec, name); err != nil {
			return err
		}
	} else {
		requiredPath := filepath.Join(liveDir, spec.required)
		if _, err := os.Stat(requiredPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%s is not currently logged in: missing %s", spec.name, requiredPath)
			}
			return err
		}
		slotDir, err := slotDir(spec.name, name)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(slotDir); err != nil {
			return err
		}
		if err := os.MkdirAll(slotDir, 0o755); err != nil {
			return err
		}
		copied := 0
		for _, rel := range spec.files {
			src := filepath.Join(liveDir, rel)
			ok, err := copyIfExists(src, filepath.Join(slotDir, rel))
			if err != nil {
				return err
			}
			if ok {
				copied++
			}
		}
		if copied == 0 {
			return fmt.Errorf("no %s account files found in %s", spec.name, liveDir)
		}
	}
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	if reg.Active == nil {
		reg.Active = map[string]string{}
	}
	reg.Active[spec.name] = name
	return saveRegistry(reg)
}

func Use(provider, name string) error {
	spec, err := resolveProvider(provider)
	if err != nil {
		return err
	}
	if err := validateName(name); err != nil {
		return err
	}
	slotDir, err := slotDir(spec.name, name)
	if err != nil {
		return err
	}
	requiredSnapshot := filepath.Join(slotDir, spec.required)
	if _, err := os.Stat(requiredSnapshot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s account %q not found", spec.name, name)
		}
		return err
	}
	if strings.TrimSpace(spec.slotHomeEnvVar) != "" {
		return saveActiveSlot(spec.name, name)
	}
	liveDir := spec.liveDir()
	if strings.TrimSpace(liveDir) == "" {
		return fmt.Errorf("%s live config dir is empty", spec.name)
	}
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	liveState, err := codexLiveState(spec, reg, name)
	if err != nil {
		return err
	}
	if liveState == codexLiveStateUnknown {
		if err := syncCurrentActiveSlot(spec, reg, name); err != nil {
			return err
		}
	} else {
		switch liveState {
		case codexLiveStateCurrent:
			if strings.TrimSpace(reg.Active[spec.name]) == name {
				if err := syncSlotFromLive(spec, name); err != nil {
					return err
				}
				return saveActiveSlot(spec.name, name)
			}
			if err := syncCurrentActiveSlot(spec, reg, name); err != nil {
				return err
			}
		case codexLiveStateTarget:
			if err := saveActiveSlot(spec.name, name); err != nil {
				return err
			}
			return nil
		case codexLiveStateConflict:
			return fmt.Errorf("%s live auth does not match saved slot state; run `sb account save %s <name>` after logging into the desired account", spec.name, spec.name)
		}
	}
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		return err
	}
	for _, rel := range spec.files {
		src := filepath.Join(slotDir, rel)
		ok, err := copyIfExists(src, filepath.Join(liveDir, rel))
		if err != nil {
			return err
		}
		if !ok && rel == spec.required {
			return fmt.Errorf("%s account %q is missing required file %s", spec.name, name, rel)
		}
	}
	if reg.Active == nil {
		reg.Active = map[string]string{}
	}
	reg.Active[spec.name] = name
	return saveRegistry(reg)
}

func List() ([]Slot, error) {
	root, err := accountsRoot()
	if err != nil {
		return nil, err
	}
	reg, err := loadRegistry()
	if err != nil {
		return nil, err
	}
	var slots []Slot
	for _, provider := range []string{"claude", "codex"} {
		dir := filepath.Join(root, provider)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			slots = append(slots, Slot{
				Provider: provider,
				Name:     entry.Name(),
				SavedAt:  info.ModTime(),
				Active:   reg.Active[provider] == entry.Name(),
			})
		}
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].Provider != slots[j].Provider {
			return slots[i].Provider < slots[j].Provider
		}
		return slots[i].Name < slots[j].Name
	})
	return slots, nil
}

func Show() (Status, error) {
	reg, err := loadRegistry()
	if err != nil {
		return Status{}, err
	}
	if reg.Active == nil {
		reg.Active = map[string]string{}
	}
	return Status{Active: reg.Active}, nil
}

func accountsRoot() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "accounts"), nil
}

func ensureRoot() error {
	root, err := accountsRoot()
	if err != nil {
		return err
	}
	return os.MkdirAll(root, 0o755)
}

func slotDir(provider, name string) (string, error) {
	root, err := accountsRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, provider, name), nil
}

func registryPath() (string, error) {
	root, err := accountsRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "registry.json"), nil
}

func loadRegistry() (registry, error) {
	path, err := registryPath()
	if err != nil {
		return registry{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return registry{Active: map[string]string{}}, nil
		}
		return registry{}, err
	}
	var reg registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return registry{}, err
	}
	if reg.Active == nil {
		reg.Active = map[string]string{}
	}
	return reg, nil
}

func saveRegistry(reg registry) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func validateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("account name is required")
	}
	if name == "." || name == ".." || name != filepath.Base(name) || strings.Contains(name, string(filepath.Separator)) {
		return fmt.Errorf("invalid account name %q", name)
	}
	return nil
}

func resolveProvider(provider string) (providerSpec, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "claude":
		return providerSpec{
			name:     "claude",
			liveDir:  claudeLiveDir,
			required: ".credentials.json",
			files:    []string{".credentials.json"},
		}, nil
	case "codex":
		return providerSpec{
			name:           "codex",
			liveDir:        codexLiveDir,
			required:       "auth.json",
			files:          []string{"auth.json", "logs_2.sqlite"},
			slotHomeEnvVar: "CODEX_HOME",
			snapshotWhole:  true,
		}, nil
	default:
		return providerSpec{}, fmt.Errorf("unsupported provider %q (want claude or codex)", provider)
	}
}

func claudeLiveDir() string {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

func codexLiveDir() string {
	if v := strings.TrimSpace(os.Getenv("CODEX_HOME")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

func copyIfExists(src, dst string) (bool, error) {
	if filepath.Clean(src) == filepath.Clean(dst) {
		return true, nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func replaceDirContents(srcDir, dstDir string) error {
	srcDir = filepath.Clean(srcDir)
	dstDir = filepath.Clean(dstDir)
	if srcDir == dstDir {
		return nil
	}
	if err := os.RemoveAll(dstDir); err != nil {
		return err
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(dstPath, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode().Perm())
	})
}

func syncSlotFromLiveHome(spec providerSpec, slotName string) error {
	liveDir := spec.liveDir()
	if strings.TrimSpace(liveDir) == "" {
		return fmt.Errorf("%s live config dir is empty", spec.name)
	}
	requiredPath := filepath.Join(liveDir, spec.required)
	if _, err := os.Stat(requiredPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s is not currently logged in: missing %s", spec.name, requiredPath)
		}
		return err
	}
	slotDir, err := slotDir(spec.name, slotName)
	if err != nil {
		return err
	}
	return replaceDirContents(liveDir, slotDir)
}

func ActiveRuntimeDir(provider string) (string, bool, error) {
	spec, err := resolveProvider(provider)
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(spec.slotHomeEnvVar) == "" {
		return "", false, nil
	}
	reg, err := loadRegistry()
	if err != nil {
		return "", false, err
	}
	name := strings.TrimSpace(reg.Active[spec.name])
	if name == "" {
		return "", false, nil
	}
	dir, err := slotDir(spec.name, name)
	if err != nil {
		return "", false, err
	}
	if _, err := os.Stat(filepath.Join(dir, spec.required)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	return dir, true, nil
}

func ActiveEnv(provider string) ([]string, error) {
	spec, err := resolveProvider(provider)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(spec.slotHomeEnvVar) == "" {
		return nil, nil
	}
	dir, ok, err := ActiveRuntimeDir(provider)
	if err != nil || !ok {
		return nil, err
	}
	return []string{spec.slotHomeEnvVar + "=" + dir}, nil
}

func PrepareLogin(provider, name string) (string, error) {
	spec, err := resolveProvider(provider)
	if err != nil {
		return "", err
	}
	if err := validateName(name); err != nil {
		return "", err
	}
	if strings.TrimSpace(spec.slotHomeEnvVar) == "" {
		return "", fmt.Errorf("%s does not support isolated account login", spec.name)
	}
	dir, err := slotDir(spec.name, name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

type codexLiveStateKind int

const (
	codexLiveStateUnknown codexLiveStateKind = iota
	codexLiveStateCurrent
	codexLiveStateTarget
	codexLiveStateConflict
)

func syncCurrentActiveSlot(spec providerSpec, reg registry, targetName string) error {
	currentName := strings.TrimSpace(reg.Active[spec.name])
	if currentName == "" || currentName == targetName {
		return nil
	}
	return syncSlotFromLive(spec, currentName)
}

func syncSlotFromLive(spec providerSpec, slotName string) error {
	liveDir := spec.liveDir()
	if strings.TrimSpace(liveDir) == "" {
		return fmt.Errorf("%s live config dir is empty", spec.name)
	}
	requiredPath := filepath.Join(liveDir, spec.required)
	if _, err := os.Stat(requiredPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	slotDir, err := slotDir(spec.name, slotName)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(slotDir); err != nil {
		return err
	}
	if err := os.MkdirAll(slotDir, 0o755); err != nil {
		return err
	}
	copied := 0
	for _, rel := range spec.files {
		src := filepath.Join(liveDir, rel)
		ok, err := copyIfExists(src, filepath.Join(slotDir, rel))
		if err != nil {
			return err
		}
		if ok {
			copied++
		}
	}
	if copied == 0 {
		return fmt.Errorf("no %s account files found in %s", spec.name, liveDir)
	}
	return nil
}

func codexLiveState(spec providerSpec, reg registry, targetName string) (codexLiveStateKind, error) {
	if spec.name != "codex" {
		return codexLiveStateUnknown, nil
	}
	liveIdentity, ok, err := readCodexIdentity(filepath.Join(spec.liveDir(), spec.required))
	if err != nil || !ok {
		return codexLiveStateUnknown, err
	}
	targetIdentity, ok, err := readCodexIdentityFromSlot(spec.name, targetName, spec.required)
	if err != nil || !ok {
		return codexLiveStateUnknown, err
	}
	currentName := strings.TrimSpace(reg.Active[spec.name])
	if currentName != "" {
		currentIdentity, ok, err := readCodexIdentityFromSlot(spec.name, currentName, spec.required)
		if err != nil {
			return codexLiveStateUnknown, err
		}
		if ok && sameIdentity(liveIdentity, currentIdentity) {
			return codexLiveStateCurrent, nil
		}
	}
	if sameIdentity(liveIdentity, targetIdentity) {
		return codexLiveStateTarget, nil
	}
	return codexLiveStateConflict, nil
}

func saveActiveSlot(provider, name string) error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	if reg.Active == nil {
		reg.Active = map[string]string{}
	}
	reg.Active[provider] = name
	return saveRegistry(reg)
}

func readCodexIdentityFromSlot(provider, name, required string) (authIdentity, bool, error) {
	dir, err := slotDir(provider, name)
	if err != nil {
		return authIdentity{}, false, err
	}
	return readCodexIdentity(filepath.Join(dir, required))
}

func readCodexIdentity(path string) (authIdentity, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return authIdentity{}, false, nil
		}
		return authIdentity{}, false, err
	}
	var raw struct {
		Tokens struct {
			AccountID string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return authIdentity{}, false, nil
	}
	id := strings.TrimSpace(raw.Tokens.AccountID)
	if id == "" {
		return authIdentity{}, false, nil
	}
	return authIdentity{AccountID: id}, true, nil
}

func sameIdentity(a, b authIdentity) bool {
	return strings.TrimSpace(a.AccountID) != "" && a.AccountID == b.AccountID
}
