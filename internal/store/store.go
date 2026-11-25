package store

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"go-atomcode-switch-cli/internal/models"

	"gopkg.in/yaml.v3"
)

const (
	accountsFileName = "atomcode-accounts.yaml"
	localConfigName  = "config.yaml"
)

var exeDirForTest string

// LocalConfig 是 exe 旁 config.yaml 的内容。
type LocalConfig struct {
	CustomDataDir string `yaml:"custom_data_dir,omitempty"`
}

// HomeDir 返回用户主目录，Windows 下优先 USERPROFILE。
func HomeDir() string {
	var primary, fallback string
	if runtime.GOOS == "windows" {
		primary, fallback = os.Getenv("USERPROFILE"), os.Getenv("HOME")
	} else {
		primary, fallback = os.Getenv("HOME"), os.Getenv("USERPROFILE")
	}
	for _, v := range []string{primary, fallback} {
		if v != "" {
			return v
		}
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return "."
}

func exeDir() string {
	if exeDirForTest != "" {
		return exeDirForTest
	}
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); dir != "" {
			return dir
		}
	}
	return "."
}

// LocalConfigPath 返回 exe 旁 config.yaml 的路径。
func LocalConfigPath() string {
	return filepath.Join(exeDir(), localConfigName)
}

// BackupFile 为已存在文件写入 .bak 副本。
func BackupFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak", data, 0o600)
}

// AtomicWriteText 使用 .tmp + fsync + rename 原子写入文本文件。
// keepBackup=true 时先保留 .bak；auth.toml 写入应传 false。
func AtomicWriteText(path, content string, keepBackup bool) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if keepBackup {
		_ = BackupFile(path)
	}
	if err := os.Rename(tmp, path); err != nil {
		if werr := os.WriteFile(path, []byte(content), 0o600); werr != nil {
			_ = os.Remove(tmp)
			return werr
		}
		_ = os.Remove(tmp)
	}
	return nil
}

// Store 绑定一个数据目录。
type Store struct {
	dataDir string
}

// New 创建 Store。
func New(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// DataDir 返回绑定的数据目录。
func (s *Store) DataDir() string {
	return s.dataDir
}

// AccountsPath 返回账号配置文件路径。
func (s *Store) AccountsPath() string {
	return filepath.Join(s.dataDir, accountsFileName)
}

// Load 读取配置；解析失败会备份原文件并返回默认配置与错误。
func (s *Store) Load() (*models.AppConfig, error) {
	s.migrateLegacyIfNeeded()
	path := s.AccountsPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return models.DefaultConfig(), nil
	}
	if err != nil {
		return models.DefaultConfig(), fmt.Errorf("read %s: %w", path, err)
	}
	var cfg models.AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		_ = BackupFile(path)
		return models.DefaultConfig(), fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.Normalize()
	return &cfg, nil
}

// Save 保存账号配置。
func (s *Store) Save(cfg *models.AppConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return AtomicWriteText(s.AccountsPath(), string(data), true)
}

// Export 导出账号配置。
func (s *Store) Export(path string, cfg *models.AppConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return AtomicWriteText(path, string(data), true)
}

// Import 读取并校验导入文件，不修改当前配置。
func (s *Store) Import(path string) (*models.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg models.AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.Normalize()
	return &cfg, nil
}

func (s *Store) migrateLegacyIfNeeded() {
	oldPath := filepath.Join(HomeDir(), ".atomcode-switch", "config.yaml")
	if _, err := os.Stat(oldPath); err != nil {
		return
	}
	newPath := s.AccountsPath()
	if _, err := os.Stat(newPath); err == nil {
		return
	}
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return
	}
	if len(bytes.TrimSpace(data)) == 0 {
		_ = os.Remove(oldPath)
		return
	}
	var probe models.AppConfig
	if yaml.Unmarshal(data, &probe) == nil {
		if AtomicWriteText(newPath, string(data), false) == nil {
			_ = os.Remove(oldPath)
		}
	} else {
		_ = BackupFile(oldPath)
	}
}

// ResolveDataDir 按 flag、环境变量、exe 旁 config.yaml、默认目录的优先级解析数据目录。
func ResolveDataDir(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv("ATOMCODE_SWITCH_DATA_DIR"); v != "" {
		return v
	}
	if dir := loadLocalConfig().CustomDataDir; dir != "" {
		return dir
	}
	return filepath.Join(HomeDir(), ".atomcode-switch")
}

// LoadLocalConfig 读取 exe 旁 config.yaml；解析失败会备份并回退默认值。
func LoadLocalConfig() LocalConfig {
	return loadLocalConfig()
}

func loadLocalConfig() LocalConfig {
	path := LocalConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return LocalConfig{}
	}
	var lc LocalConfig
	if err := yaml.Unmarshal(data, &lc); err != nil {
		_ = BackupFile(path)
		return LocalConfig{}
	}
	return lc
}

// SaveLocalConfig 保存或清除 exe 旁 config.yaml 的自定义数据目录。
func SaveLocalConfig(dir string) error {
	path := LocalConfigPath()
	if dir == "" {
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		return os.Remove(path)
	}
	lc := loadLocalConfig()
	lc.CustomDataDir = dir
	data, err := yaml.Marshal(&lc)
	if err != nil {
		return err
	}
	return AtomicWriteText(path, string(data), true)
}
