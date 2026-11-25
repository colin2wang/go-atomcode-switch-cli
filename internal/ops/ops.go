package ops

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go-atomcode-switch-cli/internal/atomio"
	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/models"
	"go-atomcode-switch-cli/internal/parser"
	"go-atomcode-switch-cli/internal/store"
)

var (
	ErrNotFound       = errors.New("account not found")
	ErrNoFields       = errors.New("no recognizable account fields")
	ErrUserUnmatched  = errors.New("login user does not match any account")
	ErrNoActive       = errors.New("no active account")
	ErrActiveNotFound = errors.New("active account not found in account list")
	ErrAuthNotFound   = errors.New("auth.toml not found")
)

const hysteresis = 5.0

// Service 封装账号业务事务。
type Service struct {
	Cfg     *models.AppConfig
	Store   *store.Store
	DataDir string
}

// New 创建业务服务。
func New(cfg *models.AppConfig, st *store.Store) *Service {
	return &Service{Cfg: cfg, Store: st, DataDir: st.DataDir()}
}

// Save 持久化当前配置。
func (s *Service) Save() error {
	return s.Store.Save(s.Cfg)
}

// FindByID 按 ID 查找账号。
func (s *Service) FindByID(id string) *models.ManagedAccount {
	for i := range s.Cfg.Accounts {
		if s.Cfg.Accounts[i].ID == id {
			return &s.Cfg.Accounts[i]
		}
	}
	return nil
}

// Active 返回当前激活账号。
func (s *Service) Active() *models.ManagedAccount {
	return s.FindByID(s.Cfg.ActiveID())
}

// Resolve 支持 1-based 序号、账号 ID、名称和用户名。
func (s *Service) Resolve(pattern string) (*models.ManagedAccount, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, ErrNotFound
	}
	if n, err := strconv.Atoi(pattern); err == nil {
		if n >= 1 && n <= len(s.Cfg.Accounts) {
			return &s.Cfg.Accounts[n-1], nil
		}
		return nil, fmt.Errorf("%w: index %d (1..%d)", ErrNotFound, n, len(s.Cfg.Accounts))
	}
	for i := range s.Cfg.Accounts {
		acc := &s.Cfg.Accounts[i]
		if acc.ID == pattern || acc.AuthData.User.Name == pattern || acc.AuthData.User.Username == pattern {
			return acc, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrNotFound, pattern)
}

// Switch 写入目标账号 auth.toml 并更新激活账号。
func (s *Service) Switch(id string) error {
	acc := s.FindByID(id)
	if acc == nil {
		return ErrNotFound
	}
	if err := atomio.Write(s.Cfg.AtomcodeDir(), &acc.AuthData); err != nil {
		return fmt.Errorf("write auth.toml: %w", err)
	}
	s.Cfg.SetActiveID(id)
	return s.Save()
}

// SyncCurrentAuthExisting 仅同步已存在账号的凭据，不会新增账号。
func (s *Service) SyncCurrentAuthExisting() error {
	auth, err := atomio.Read(s.Cfg.AtomcodeDir())
	if err != nil {
		return nil
	}
	id := auth.User.ID
	for i := range s.Cfg.Accounts {
		if s.Cfg.Accounts[i].ID != id {
			continue
		}
		s.Cfg.Accounts[i].AuthData = *auth
		s.Cfg.SetActiveID(id)
		return s.Save()
	}
	return nil
}

// ImportCurrentAuth 从当前 auth.toml 新增或刷新账号，并设为激活。
func (s *Service) ImportCurrentAuth() (isNew bool, name string, err error) {
	auth, err := atomio.Read(s.Cfg.AtomcodeDir())
	if err != nil {
		return false, "", ErrAuthNotFound
	}
	id := auth.User.ID
	for i := range s.Cfg.Accounts {
		if s.Cfg.Accounts[i].ID != id {
			continue
		}
		s.Cfg.Accounts[i].AuthData = *auth
		s.Cfg.Accounts[i].LastUpdated = NowStr()
		s.Cfg.SetActiveID(id)
		return false, auth.User.Name, s.Save()
	}
	s.Cfg.Accounts = append(s.Cfg.Accounts, models.ManagedAccount{
		ID:           id,
		AuthData:     *auth,
		PlanName:     i18n.T("plan_unknown"),
		UsagePercent: 0,
		ResetTime:    DefaultResetTime(),
		IsValid:      true,
		LastUpdated:  NowStr(),
		InfoStatus:   models.StatusUnknown,
	})
	s.Cfg.SetActiveID(id)
	return true, auth.User.Name, s.Save()
}

// Delete 删除账号；删除激活账号后自动切换到第一个剩余账号。
func (s *Service) Delete(id string) error {
	idx := -1
	for i := range s.Cfg.Accounts {
		if s.Cfg.Accounts[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	s.Cfg.Accounts = append(s.Cfg.Accounts[:idx], s.Cfg.Accounts[idx+1:]...)
	if s.Cfg.ActiveID() == id {
		s.Cfg.ActiveAccountID = nil
		if len(s.Cfg.Accounts) > 0 {
			return s.Switch(s.Cfg.Accounts[0].ID)
		}
	}
	return s.Save()
}

// CheckAutoSwitch 按阈值、5% 滞回和 Fresh 限定检查并执行自动切换。
func (s *Service) CheckAutoSwitch() (string, bool) {
	r := &s.Cfg.AutoSwitchRules
	if !r.Enabled {
		return "", false
	}
	active := s.Active()
	if active == nil || active.UsagePercent < r.MaxUsagePercent {
		return "", false
	}

	var best *models.ManagedAccount
	for i := range s.Cfg.Accounts {
		acc := &s.Cfg.Accounts[i]
		if acc.ID == active.ID || !acc.IsValid || acc.InfoStatus != models.StatusFresh {
			continue
		}
		if acc.UsagePercent >= r.MaxUsagePercent {
			continue
		}
		if active.UsagePercent-acc.UsagePercent < hysteresis {
			continue
		}
		if best == nil || acc.UsagePercent < best.UsagePercent {
			best = acc
		}
	}
	if best == nil {
		return "", false
	}

	id, name := best.ID, best.AuthData.User.Name
	if err := s.Switch(id); err != nil {
		slog.Warn("auto switch failed", "err", err)
		return "", false
	}
	return name, true
}

// ApplyLoginOutput 解析 /login 输出并事务性更新目标账号。
func (s *Service) ApplyLoginOutput(text string) (string, error) {
	active := s.Active()
	if active == nil {
		if s.Cfg.ActiveID() == "" {
			return "", ErrNoActive
		}
		return "", ErrActiveNotFound
	}

	target := active
	if loginName := ExtractLoginName(text); loginName != "" && !matchesLoginUser(&active.AuthData.User, loginName) {
		found := s.findByLoginName(loginName)
		if found == nil {
			return "", fmt.Errorf("%w: %s", ErrUserUnmatched, loginName)
		}
		target = found
	}

	fields := parser.Parse(text, time.Now())
	if fields.Empty() {
		return "", ErrNoFields
	}
	if fields.HasPlan {
		target.PlanName = fields.PlanName
	}
	if fields.HasUsage {
		target.UsagePercent = fields.UsagePercent
	}
	if fields.HasReset {
		target.ResetTime = fields.ResetTime
	}
	if fields.HasDays {
		target.RemainingDays = fields.RemainingDays
	}
	target.InfoStatus = models.StatusFresh
	target.IsValid = true
	target.LastUpdated = NowStr()

	if target != active {
		if err := atomio.Write(s.Cfg.AtomcodeDir(), &target.AuthData); err != nil {
			return "", fmt.Errorf("write auth.toml: %w", err)
		}
		s.Cfg.SetActiveID(target.ID)
	}
	if err := s.Save(); err != nil {
		return "", err
	}
	return target.AuthData.User.Name, nil
}

func (s *Service) findByLoginName(name string) *models.ManagedAccount {
	for i := range s.Cfg.Accounts {
		if matchesLoginUser(&s.Cfg.Accounts[i].AuthData.User, name) {
			return &s.Cfg.Accounts[i]
		}
	}
	return nil
}

func matchesLoginUser(user *models.User, name string) bool {
	return user.Name == name || user.Username == name
}

// ExtractLoginName 兼容中英文登录用户标记。
func ExtractLoginName(text string) string {
	markers := []string{"logged in as ", "已登录为", "登录为", "当前用户"}
	for _, marker := range markers {
		pos := strings.Index(text, marker)
		if pos < 0 {
			continue
		}
		rest := text[pos+len(marker):]
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
		rest = strings.TrimLeft(rest, ":：")
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
		if i := strings.IndexAny(rest, "\r\n"); i >= 0 {
			rest = rest[:i]
		}
		end := len(rest)
		for i, r := range rest {
			if unicode.IsSpace(r) || strings.ContainsRune("。（(，,:：", r) {
				end = i
				break
			}
		}
		if name := strings.TrimSpace(rest[:end]); name != "" {
			return name
		}
	}
	return ""
}

// ImportFile 事务性导入账号、激活账号和自动切换规则，保留其余本地配置。
func (s *Service) ImportFile(path string) (int, error) {
	imported, err := s.Store.Import(path)
	if err != nil {
		return 0, err
	}
	s.Cfg.Accounts = imported.Accounts
	if imported.ActiveAccountID != nil {
		id := *imported.ActiveAccountID
		s.Cfg.ActiveAccountID = &id
	} else {
		s.Cfg.ActiveAccountID = nil
	}
	s.Cfg.AutoSwitchRules = imported.AutoSwitchRules
	return len(s.Cfg.Accounts), s.Save()
}

// NowStr 返回当前本地时间字符串。
func NowStr() string {
	return time.Now().Format("2006-01-02 15:04")
}

// DefaultResetTime 返回当前时间加一小时的完整时间字符串。
func DefaultResetTime() string {
	return time.Now().Add(time.Hour).Format("2006-01-02 15:04")
}
