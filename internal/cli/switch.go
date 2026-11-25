package cli

import (
	"fmt"
	"time"

	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/timeutil"

	"github.com/spf13/cobra"
)

func newSwitchCmd() *cobra.Command {
	var doUpdate bool
	cmd := &cobra.Command{
		Use:   "switch <index|id|name>",
		Short: "Switch active account (writes auth.toml)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := bootstrap()
			acc, err := svc.Resolve(args[0])
			if err != nil {
				return err
			}
			if err := svc.Switch(acc.ID); err != nil {
				return err
			}
			fmt.Println(i18n.T("switched", acc.AuthData.User.Name))
			if timeutil.IsExpired(acc.ResetTime, time.Now()) {
				fmt.Println(i18n.T("switch_expired_hint", acc.AuthData.User.Name))
				if doUpdate || svc.Cfg.AutoUpdateOnSwitchExpired {
					return runUpdate(svc, 10*time.Second, "", "", false)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&doUpdate, "update", false, "run update when the target account is expired")
	return cmd
}
