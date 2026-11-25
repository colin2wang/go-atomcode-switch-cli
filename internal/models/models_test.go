package models

import (
	"bytes"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

func TestOldConfigMissingFieldsLoadsWithDefaults(t *testing.T) {
	data := "custom_atomcode_dir: null\nactive_account_id: null\n"
	var cfg AppConfig
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	cfg.Normalize()
	if len(cfg.Accounts) != 0 {
		t.Fatalf("accounts = %d, want 0", len(cfg.Accounts))
	}
	if cfg.Language != "zh_cn" {
		t.Fatalf("language = %q, want zh_cn", cfg.Language)
	}
	if cfg.AutoUpdateOnSwitchExpired {
		t.Fatal("auto_update_on_switch_expired = true, want false")
	}
	if cfg.SortMode != SortNone {
		t.Fatalf("sort_mode = %q, want None", cfg.SortMode)
	}
	if cfg.AutoSwitchRules.Enabled {
		t.Fatal("auto switch enabled = true, want false")
	}
	if cfg.AutoSwitchRules.MaxUsagePercent != 95 {
		t.Fatalf("threshold = %v, want 95", cfg.AutoSwitchRules.MaxUsagePercent)
	}
}

func TestAccountMissingInfoStatusDefaultsToUnknown(t *testing.T) {
	data := `id: abc
plan_name: Old
usage_percent: 10.0
reset_time: "09:00"
is_valid: true
remaining_days: 5
last_updated: x
auth_data:
  access_token: a
  refresh_token: r
  token_type: Bearer
  expires_in: 1
  created_at: 1
  user:
    id: u
    username: n
    name: n
    email: e
`
	var acc ManagedAccount
	if err := yaml.Unmarshal([]byte(data), &acc); err != nil {
		t.Fatalf("unmarshal account: %v", err)
	}
	if acc.InfoStatus != StatusUnknown {
		t.Fatalf("info_status = %q, want Unknown", acc.InfoStatus)
	}
	if acc.ResetTime != "09:00" {
		t.Fatalf("reset_time = %q, want 09:00", acc.ResetTime)
	}
}

func TestAuthTomlWithoutAvatarURL(t *testing.T) {
	content := `
access_token = "..."
refresh_token = "..."
token_type = "Bearer"
expires_in = 604799
created_at = 1795087094

[user]
id = "u"
username = "n"
name = "n"
email = "e@example.com"
`
	var auth AuthToml
	if _, err := toml.Decode(content, &auth); err != nil {
		t.Fatalf("decode auth: %v", err)
	}
	if auth.User.AvatarURL != "" {
		t.Fatalf("avatar_url = %q, want empty", auth.User.AvatarURL)
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(&auth); err != nil {
		t.Fatalf("encode auth: %v", err)
	}
	if strings.Contains(buf.String(), "avatar_url") {
		t.Fatalf("encoded auth contains avatar_url: %s", buf.String())
	}
}

func TestSortModeAndInfoStatusUnmarshalCompatibility(t *testing.T) {
	var cfg struct {
		Sort  SortMode   `yaml:"sort"`
		Info  InfoStatus `yaml:"info"`
		Sort2 SortMode   `yaml:"sort2"`
		Info2 InfoStatus `yaml:"info2"`
		Sort3 SortMode   `yaml:"sort3"`
		Info3 InfoStatus `yaml:"info3"`
	}
	data := "sort: reset_time\ninfo: fresh\nsort2: WindowRemaining\ninfo2: unknown\nsort3: nonsense\ninfo3: nonsense\n"
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("unmarshal enums: %v", err)
	}
	if cfg.Sort != SortResetTime || cfg.Sort2 != SortWindowRemaining || cfg.Sort3 != SortNone {
		t.Fatalf("unexpected sort modes: %q %q %q", cfg.Sort, cfg.Sort2, cfg.Sort3)
	}
	if cfg.Info != StatusFresh || cfg.Info2 != StatusUnknown || cfg.Info3 != StatusUnknown {
		t.Fatalf("unexpected info statuses: %q %q %q", cfg.Info, cfg.Info2, cfg.Info3)
	}
}

func TestNormalizeDoesNotOverwriteLegalValues(t *testing.T) {
	cfg := &AppConfig{
		Language:        "en_us",
		SortMode:        SortResetTime,
		AutoSwitchRules: AutoSwitchRules{Enabled: true, MaxUsagePercent: 60},
	}
	cfg.Normalize()
	if cfg.Language != "en_us" || cfg.SortMode != SortResetTime || cfg.AutoSwitchRules.MaxUsagePercent != 60 {
		t.Fatalf("normalize overwrote legal values: %+v", cfg)
	}
}
