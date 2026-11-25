package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-atomcode-switch-cli/internal/i18n"

	"github.com/spf13/cobra"
)

func newDaemonCmd() *cobra.Command {
	var interval time.Duration
	var once bool
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Periodically check auto-switch rules (use --once for cron)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval <= 0 {
				return errors.New(i18n.T("invalid_interval"))
			}
			svc := bootstrap()
			if err := svc.SyncCurrentAuthExisting(); err != nil {
				slog.Warn("sync current auth failed", "err", err)
			}
			check := func() {
				if name, switched := svc.CheckAutoSwitch(); switched {
					fmt.Println(time.Now().Format(time.RFC3339), i18n.T("auto_switched", name))
				}
			}
			check()
			if once {
				return nil
			}

			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(sig)
			slog.Info(i18n.T("daemon_started", interval.String()))
			for {
				select {
				case <-sig:
					slog.Info(i18n.T("daemon_stopped"))
					return nil
				case <-ticker.C:
					check()
				}
			}
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Second, "check interval")
	cmd.Flags().BoolVar(&once, "once", false, "run one check and exit (for cron)")
	return cmd
}
