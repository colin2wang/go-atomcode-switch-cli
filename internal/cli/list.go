package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/models"
	"go-atomcode-switch-cli/internal/timeutil"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all accounts",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			printAccounts(os.Stdout, bootstrap().Cfg)
			return nil
		},
	}
}

// printAccounts 渲染账号表格，CLI list 与交互菜单共用。
func printAccounts(w io.Writer, cfg *models.AppConfig) {
	if len(cfg.Accounts) == 0 {
		fmt.Fprintln(w, i18n.T("list_empty"))
		return
	}
	sortAccounts(cfg)
	now := time.Now()
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tACTIVE\tNAME\tUSER\tPLAN\tUSAGE\tRESET\tSTATUS")
	for i := range cfg.Accounts {
		a := &cfg.Accounts[i]
		mark := ""
		if a.ID == cfg.ActiveID() {
			mark = "*"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%.0f%%\t%s\t%s\n",
			i+1, mark, a.AuthData.User.Name, a.AuthData.User.Username,
			a.PlanName, a.UsagePercent, resetCol(a, now), stateOf(a, now))
	}
	_ = tw.Flush()
}

func stateOf(a *models.ManagedAccount, now time.Time) string {
	switch {
	case !a.IsValid:
		return "INVALID"
	case timeutil.IsExpired(a.ResetTime, now):
		return "EXPIRED"
	case timeutil.IsExpiringSoon(a.ResetTime, now):
		return "EXPIRING"
	default:
		return "OK"
	}
}

func resetCol(a *models.ManagedAccount, now time.Time) string {
	t, ok := timeutil.ParseResetDatetime(a.ResetTime, now)
	if !ok {
		return a.ResetTime
	}
	d := t.Sub(now)
	if d <= 0 {
		return "overdue"
	}
	if d >= time.Hour {
		return fmt.Sprintf("in %dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("in %dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func sortAccounts(cfg *models.AppConfig) {
	now := time.Now()
	activeID := cfg.ActiveID()
	isActive := func(a *models.ManagedAccount) bool { return a.ID == activeID }
	stale := func(a *models.ManagedAccount) bool {
		t, ok := timeutil.ParseResetDatetime(a.ResetTime, now)
		return !ok || t.Before(now)
	}
	remaining := func(a *models.ManagedAccount) float64 { return 100 - float64(a.UsagePercent) }
	sort.SliceStable(cfg.Accounts, func(i, j int) bool {
		a, b := &cfg.Accounts[i], &cfg.Accounts[j]
		if cfg.SortMode == models.SortNone {
			return a.AuthData.User.Name < b.AuthData.User.Name
		}
		if ai, bi := isActive(a), isActive(b); ai != bi {
			return ai
		}
		if sa, sb := stale(a), stale(b); sa != sb {
			return !sa
		}
		ta, _ := timeutil.ParseResetDatetime(a.ResetTime, now)
		tb, _ := timeutil.ParseResetDatetime(b.ResetTime, now)
		ra, rb := remaining(a), remaining(b)
		if cfg.SortMode == models.SortResetTime {
			if !ta.Equal(tb) {
				return ta.Before(tb)
			}
			return rb < ra
		}
		if ra != rb {
			return ra > rb
		}
		return ta.Before(tb)
	})
}
