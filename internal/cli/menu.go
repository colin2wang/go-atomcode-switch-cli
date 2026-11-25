package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/models"
	"go-atomcode-switch-cli/internal/ops"
	"go-atomcode-switch-cli/internal/timeutil"

	"github.com/spf13/cobra"
)

func newMenuCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "menu",
		Aliases: []string{"m", "ui"},
		Short:   "Interactive menu mode (arrow keys + Enter)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMenu()
		},
	}
}

func runMenu() error {
	if !stdinIsTTY() {
		return errors.New(i18n.T("menu_requires_tty"))
	}
	t, err := newTTY()
	if err != nil {
		return errors.New(i18n.T("menu_requires_tty"))
	}
	defer t.restore()

	for {
		svc := bootstrap()
		idx := t.choose(i18n.T("menu_title"), menuItems(svc), "")
		switch idx {
		case -1, 9:
			return nil
		case 0:
			fmt.Print(ansiClear)
			printAccounts(os.Stdout, svc.Cfg)
			t.pause()
		case 1:
			menuSwitch(t, svc)
		case 2:
			menuUpdate(t, svc)
		case 3:
			menuSync(t, svc)
		case 4:
			menuAutoSwitch(t, svc)
		case 5:
			menuDelete(t, svc)
		case 6:
			menuExport(t, svc)
		case 7:
			menuImportFile(t, svc)
		case 8:
			menuSettings(t, svc)
		}
	}
}

func menuItems(svc *ops.Service) []string {
	active := "-"
	if acc := svc.Active(); acc != nil {
		active = acc.AuthData.User.Name
	}
	return []string{
		fmt.Sprintf(i18n.T("menu_item_list"), len(svc.Cfg.Accounts)),
		i18n.T("menu_item_switch"),
		i18n.T("menu_item_update"),
		i18n.T("menu_item_sync"),
		fmt.Sprintf(i18n.T("menu_item_auto_switch"),
			onOff(svc.Cfg.AutoSwitchRules.Enabled), svc.Cfg.AutoSwitchRules.MaxUsagePercent),
		i18n.T("menu_item_delete"),
		i18n.T("menu_item_export"),
		i18n.T("menu_item_import_file"),
		fmt.Sprintf(i18n.T("menu_item_settings"), active),
		i18n.T("menu_item_quit"),
	}
}

func onOff(b bool) string {
	if b {
		return i18n.T("menu_on")
	}
	return i18n.T("menu_off")
}

func chooseAccount(t *tty, svc *ops.Service, title string) int {
	if len(svc.Cfg.Accounts) == 0 {
		fmt.Println(i18n.T("list_empty"))
		t.pause()
		return -1
	}
	sortAccounts(svc.Cfg)
	now := time.Now()
	items := make([]string, len(svc.Cfg.Accounts))
	for i := range svc.Cfg.Accounts {
		a := &svc.Cfg.Accounts[i]
		mark := "  "
		if a.ID == svc.Cfg.ActiveID() {
			mark = "* "
		}
		items[i] = fmt.Sprintf("%s%-2d %-16s %-14s %3.0f%%  %-10s %s",
			mark, i+1, a.AuthData.User.Name, a.AuthData.User.Username,
			a.UsagePercent, resetCol(a, now), stateOf(a, now))
	}
	return t.choose(title, items, "")
}

func menuSwitch(t *tty, svc *ops.Service) {
	idx := chooseAccount(t, svc, i18n.T("menu_switch_title"))
	if idx < 0 {
		return
	}
	fmt.Print(ansiClear)
	acc := &svc.Cfg.Accounts[idx]
	if err := svc.Switch(acc.ID); err != nil {
		fmt.Println(i18n.T("switch_failed", err))
		t.pause()
		return
	}
	fmt.Println(i18n.T("switched", acc.AuthData.User.Name))
	if timeutil.IsExpired(acc.ResetTime, time.Now()) {
		fmt.Println(i18n.T("switch_expired_hint", acc.AuthData.User.Name))
		if t.confirm() {
			if err := runUpdate(svc, 10*time.Second, "", "", false); err != nil {
				fmt.Println("error:", err)
			}
		}
	}
	t.pause()
}

func menuUpdate(t *tty, svc *ops.Service) {
	fmt.Print(ansiClear)
	if err := runUpdate(svc, 10*time.Second, "", "", false); err != nil {
		fmt.Println("error:", err)
	}
	t.pause()
}

func menuSync(t *tty, svc *ops.Service) {
	fmt.Print(ansiClear)
	isNew, name, err := svc.ImportCurrentAuth()
	switch {
	case errors.Is(err, ops.ErrAuthNotFound):
		fmt.Println(i18n.T("auth_not_found"))
	case err != nil:
		fmt.Println(err)
	case isNew:
		fmt.Println(i18n.T("imported_hint", name))
	default:
		fmt.Println(i18n.T("imported_existing", name))
	}
	t.pause()
}

func menuAutoSwitch(t *tty, svc *ops.Service) {
	fmt.Print(ansiClear)
	if name, switched := svc.CheckAutoSwitch(); switched {
		fmt.Println(i18n.T("auto_switched", name))
	} else {
		fmt.Println(i18n.T("auto_switch_none"))
	}
	t.pause()
}

func menuDelete(t *tty, svc *ops.Service) {
	idx := chooseAccount(t, svc, i18n.T("menu_delete_title"))
	if idx < 0 {
		return
	}
	fmt.Print(ansiClear)
	acc := &svc.Cfg.Accounts[idx]
	if !t.confirm() {
		fmt.Println(i18n.T("delete_canceled"))
		t.pause()
		return
	}
	if err := svc.Delete(acc.ID); err != nil {
		fmt.Println(i18n.T("account_not_found", acc.AuthData.User.Name))
	} else {
		fmt.Println(i18n.T("account_deleted"))
	}
	t.pause()
}

func menuExport(t *tty, svc *ops.Service) {
	fmt.Print(ansiClear)
	path := t.readLine(i18n.T("menu_export_prompt"), "atomcode-accounts.yaml")
	if err := svc.Store.Export(path, svc.Cfg); err != nil {
		fmt.Println(i18n.T("export_failed", err))
	} else {
		fmt.Println(i18n.T("exported", path))
	}
	t.pause()
}

func menuImportFile(t *tty, svc *ops.Service) {
	fmt.Print(ansiClear)
	path := t.readLine(i18n.T("menu_import_prompt"), "")
	if path == "" {
		t.pause()
		return
	}
	n, err := svc.ImportFile(path)
	if err != nil {
		fmt.Println(i18n.T("import_file_bad", err))
	} else {
		fmt.Println(i18n.T("imported", n))
	}
	t.pause()
}

func menuSettings(t *tty, svc *ops.Service) {
	for {
		cfg := svc.Cfg
		items := []string{
			fmt.Sprintf(i18n.T("menu_settings_language"), cfg.Language),
			fmt.Sprintf(i18n.T("menu_settings_sort"), cfg.SortMode),
			fmt.Sprintf(i18n.T("menu_settings_auto_switch"), onOff(cfg.AutoSwitchRules.Enabled)),
			fmt.Sprintf(i18n.T("menu_settings_threshold"), cfg.AutoSwitchRules.MaxUsagePercent),
			i18n.T("menu_back"),
		}
		idx := t.choose(i18n.T("menu_settings_title"), items, "")
		switch idx {
		case -1, 4:
			return
		case 0:
			if cfg.Language == "en_us" {
				cfg.Language = "zh_cn"
			} else {
				cfg.Language = "en_us"
			}
			i18n.Init(cfg.Language)
			if err := svc.Save(); err != nil {
				fmt.Println(i18n.T("config_save_failed", err))
			}
		case 1:
			switch cfg.SortMode {
			case models.SortNone:
				cfg.SortMode = models.SortResetTime
			case models.SortResetTime:
				cfg.SortMode = models.SortWindowRemaining
			default:
				cfg.SortMode = models.SortNone
			}
			if err := svc.Save(); err != nil {
				fmt.Println(i18n.T("config_save_failed", err))
			}
		case 2:
			cfg.AutoSwitchRules.Enabled = !cfg.AutoSwitchRules.Enabled
			if err := svc.Save(); err != nil {
				fmt.Println(i18n.T("config_save_failed", err))
			}
		case 3:
			menuSetThreshold(t, svc)
		}
	}
}

func menuSetThreshold(t *tty, svc *ops.Service) {
	for {
		raw := t.readLine(i18n.T("menu_threshold_prompt"), "")
		if raw == "" {
			return
		}
		v, err := strconv.ParseFloat(raw, 32)
		if err != nil || v < 50 || v > 100 {
			fmt.Println(i18n.T("invalid_threshold"))
			continue
		}
		svc.Cfg.AutoSwitchRules.MaxUsagePercent = float32(v)
		if err := svc.Save(); err != nil {
			fmt.Println(i18n.T("config_save_failed", err))
		} else {
			fmt.Println(i18n.T("config_saved"))
		}
		return
	}
}
