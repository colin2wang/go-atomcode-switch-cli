package models

import (
	"math"
	"strings"

	"gopkg.in/yaml.v3"
)

// AuthToml 对应 ~/.atomcode/auth.toml。
type AuthToml struct {
	AccessToken  string `toml:"access_token" yaml:"access_token"`
	RefreshToken string `toml:"refresh_token" yaml:"refresh_token"`
	TokenType    string `toml:"token_type" yaml:"token_type"`
	ExpiresIn    uint64 `toml:"expires_in" yaml:"expires_in"`
	CreatedAt    uint64 `toml:"created_at" yaml:"created_at"`
	User         User   `toml:"user" yaml:"user"`
}

// User 是 auth.toml 中的用户信息。
type User struct {
	ID        string `toml:"id" yaml:"id"`
	Username  string `toml:"username" yaml:"username"`
	Name      string `toml:"name" yaml:"name"`
	Email     string `toml:"email" yaml:"email"`
	AvatarURL string `toml:"avatar_url,omitempty" yaml:"avatar_url,omitempty"`
}

// SortMode 保持与 Rust/serde 相同的序列化值。
type SortMode string

const (
	SortNone            SortMode = "None"
	SortResetTime       SortMode = "ResetTime"
	SortWindowRemaining SortMode = "WindowRemaining"
)

// UnmarshalYAML 兼容大小写和下划线写法，非法值回退到 None。
func (m *SortMode) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "resettime", "reset_time":
		*m = SortResetTime
	case "windowremaining", "window_remaining":
		*m = SortWindowRemaining
	default:
		*m = SortNone
	}
	return nil
}

// InfoStatus 表示账号信息是否经过 /login 解析刷新。
type InfoStatus string

const (
	StatusUnknown InfoStatus = "Unknown"
	StatusFresh   InfoStatus = "Fresh"
)

// UnmarshalYAML 兼容大小写，非法值回退到 Unknown。
func (m *InfoStatus) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(s), string(StatusFresh)) {
		*m = StatusFresh
	} else {
		*m = StatusUnknown
	}
	return nil
}

// AppConfig 对应 atomcode-accounts.yaml。
type AppConfig struct {
	CustomAtomcodeDir         *string          `yaml:"custom_atomcode_dir,omitempty"`
	ActiveAccountID           *string          `yaml:"active_account_id,omitempty"`
	Accounts                  []ManagedAccount `yaml:"accounts"`
	AutoSwitchRules           AutoSwitchRules  `yaml:"auto_switch_rules"`
	Language                  string           `yaml:"language"`
	AutoUpdateOnSwitchExpired bool             `yaml:"auto_update_on_switch_expired"`
	SortMode                  SortMode         `yaml:"sort_mode"`
}

// ManagedAccount 是受管理的账号记录。
type ManagedAccount struct {
	ID            string     `yaml:"id"`
	AuthData      AuthToml   `yaml:"auth_data"`
	PlanName      string     `yaml:"plan_name"`
	UsagePercent  float32    `yaml:"usage_percent"`
	ResetTime     string     `yaml:"reset_time"`
	IsValid       bool       `yaml:"is_valid"`
	RemainingDays uint32     `yaml:"remaining_days"`
	LastUpdated   string     `yaml:"last_updated"`
	InfoStatus    InfoStatus `yaml:"info_status"`
}

type managedAccountAlias ManagedAccount

// UnmarshalYAML 补齐旧账号缺失的 info_status 字段。
func (m *ManagedAccount) UnmarshalYAML(node *yaml.Node) error {
	var aux managedAccountAlias
	if err := node.Decode(&aux); err != nil {
		return err
	}
	*m = ManagedAccount(aux)
	if m.InfoStatus == "" {
		m.InfoStatus = StatusUnknown
	}
	return nil
}

// AutoSwitchRules 是自动切换阈值配置。
type AutoSwitchRules struct {
	Enabled         bool    `yaml:"enabled"`
	MaxUsagePercent float32 `yaml:"max_usage_percent"`
}

// DefaultConfig 返回与原 Rust AppConfig::default 一致的配置。
func DefaultConfig() *AppConfig {
	return &AppConfig{
		Accounts:                  []ManagedAccount{},
		AutoSwitchRules:           AutoSwitchRules{Enabled: false, MaxUsagePercent: 95.0},
		Language:                  "zh_cn",
		AutoUpdateOnSwitchExpired: false,
		SortMode:                  SortNone,
	}
}

// Normalize 补齐旧配置缺失字段；合法旧值保持不变。
func (c *AppConfig) Normalize() {
	if strings.TrimSpace(c.Language) == "" {
		c.Language = "zh_cn"
	}
	if !validPercent(c.AutoSwitchRules.MaxUsagePercent) {
		c.AutoSwitchRules.MaxUsagePercent = 95.0
	}
	if c.Accounts == nil {
		c.Accounts = []ManagedAccount{}
	}
	if c.SortMode == "" {
		c.SortMode = SortNone
	}
	for i := range c.Accounts {
		if c.Accounts[i].InfoStatus == "" {
			c.Accounts[i].InfoStatus = StatusUnknown
		}
	}
}

func validPercent(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v > 0 && v <= 100
}

// AtomcodeDir 返回自定义的 auth.toml 目录；空串表示使用默认 ~/.atomcode。
func (c *AppConfig) AtomcodeDir() string {
	if c.CustomAtomcodeDir == nil {
		return ""
	}
	return *c.CustomAtomcodeDir
}

// SetAtomcodeDir 设置或清除自定义 auth.toml 目录。
func (c *AppConfig) SetAtomcodeDir(dir string) {
	if dir == "" {
		c.CustomAtomcodeDir = nil
		return
	}
	c.CustomAtomcodeDir = &dir
}

// ActiveID 返回当前激活账号 ID；没有激活账号时返回空串。
func (c *AppConfig) ActiveID() string {
	if c.ActiveAccountID == nil {
		return ""
	}
	return *c.ActiveAccountID
}

// SetActiveID 设置当前激活账号 ID。
func (c *AppConfig) SetActiveID(id string) {
	c.ActiveAccountID = &id
}
