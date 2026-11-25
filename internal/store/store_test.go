package store

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go-atomcode-switch-cli/internal/models"
)

func TestAtomicWriteTextWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")
	if err := AtomicWriteText(path, "first", true); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := AtomicWriteText(path, "second", true); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "second" {
		t.Fatalf("content = %q, err = %v", got, err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != "first" {
		t.Fatalf("backup = %q, err = %v", backup, err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file remains: %v", err)
	}
}

func TestAtomicWriteTextNoBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.toml")
	if err := AtomicWriteText(path, "one", false); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := AtomicWriteText(path, "two", false); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected backup file: %v", err)
	}
}

func TestLoadCorruptConfigBacksUpAndReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	st := New(dir)
	path := st.AccountsPath()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("accounts: [unclosed"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := st.Load()
	if err == nil {
		t.Fatal("expected parse error")
	}
	if len(cfg.Accounts) != 0 || cfg.Language != "zh_cn" {
		t.Fatalf("default config not returned: %+v", cfg)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("backup not created: %v", err)
	}
}

func TestMigrateLegacyConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	dataDir := t.TempDir()
	st := New(dataDir)

	legacyDir := filepath.Join(home, ".atomcode-switch")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := "language: en_us\naccounts: []\n"
	if err := os.WriteFile(filepath.Join(legacyDir, "config.yaml"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := st.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Language != "en_us" {
		t.Fatalf("language = %q, want en_us", cfg.Language)
	}
	if _, err := os.Stat(st.AccountsPath()); err != nil {
		t.Fatalf("migrated config missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "config.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy config remains: %v", err)
	}
}

func TestResolveDataDirPriority(t *testing.T) {
	exeDir := t.TempDir()
	previous := exeDirForTest
	exeDirForTest = exeDir
	t.Cleanup(func() { exeDirForTest = previous })
	t.Setenv("ATOMCODE_SWITCH_DATA_DIR", "")

	if err := SaveLocalConfig(filepath.Join(exeDir, "local-data")); err != nil {
		t.Fatalf("save local config: %v", err)
	}
	if got := ResolveDataDir(""); got != filepath.Join(exeDir, "local-data") {
		t.Fatalf("local config dir = %q", got)
	}
	if got := ResolveDataDir(filepath.Join(exeDir, "flag-data")); got != filepath.Join(exeDir, "flag-data") {
		t.Fatalf("flag dir = %q", got)
	}
	t.Setenv("ATOMCODE_SWITCH_DATA_DIR", filepath.Join(exeDir, "env-data"))
	if got := ResolveDataDir(""); got != filepath.Join(exeDir, "env-data") {
		t.Fatalf("env dir = %q", got)
	}
	if err := SaveLocalConfig(""); err != nil {
		t.Fatalf("clear local config: %v", err)
	}
	if _, err := os.Stat(LocalConfigPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local config remains: %v", err)
	}
}

func TestLoadCorruptLocalConfigBacksUp(t *testing.T) {
	exeDir := t.TempDir()
	previous := exeDirForTest
	exeDirForTest = exeDir
	t.Cleanup(func() { exeDirForTest = previous })

	path := LocalConfigPath()
	if err := os.WriteFile(path, []byte("custom_data_dir: [bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	lc := LoadLocalConfig()
	if lc.CustomDataDir != "" {
		t.Fatalf("custom dir = %q, want empty", lc.CustomDataDir)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("local config backup missing: %v", err)
	}
}

func TestImportDoesNotModifyCurrentConfig(t *testing.T) {
	dir := t.TempDir()
	st := New(dir)
	cfg := models.DefaultConfig()
	cfg.Language = "zh_cn"
	if err := st.Save(cfg); err != nil {
		t.Fatal(err)
	}
	importPath := filepath.Join(dir, "import.yaml")
	if err := os.WriteFile(importPath, []byte("language: en_us\naccounts: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := st.Import(importPath)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.Language != "en_us" {
		t.Fatalf("imported language = %q", imported.Language)
	}
	current, err := st.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if current.Language != "zh_cn" {
		t.Fatalf("current config was modified: %q", current.Language)
	}
}

func TestHomeDirUsesPlatformPreference(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific preference")
	}
	t.Setenv("USERPROFILE", filepath.Join(t.TempDir(), "profile"))
	t.Setenv("HOME", filepath.Join(t.TempDir(), "git-bash-home"))
	if got := HomeDir(); got != os.Getenv("USERPROFILE") {
		t.Fatalf("HomeDir() = %q, want USERPROFILE", got)
	}
}
