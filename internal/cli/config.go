package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/models"
	"go-atomcode-switch-cli/internal/store"

	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or update local configuration",
	}
	cmd.AddCommand(newConfigShowCmd(), newConfigSetCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show effective configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := bootstrap()
			cfg := svc.Cfg
			active := cfg.ActiveID()
			if acc := svc.FindByID(active); acc != nil {
				active = acc.AuthData.User.Name
			}
			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, i18n.T("config_show_title"))
			fmt.Fprintf(w, "%s\t%s\n", i18n.T("config_field_data_dir"), svc.DataDir)
			fmt.Fprintf(w, "%s\t%s\n", i18n.T("config_field_atomcode_dir"), cfg.AtomcodeDir())
			fmt.Fprintf(w, "%s\t%s\n", i18n.T("config_field_language"), cfg.Language)
			fmt.Fprintf(w, "%s\t%s\n", i18n.T("config_field_sort_mode"), cfg.SortMode)
			fmt.Fprintf(w, "%s\t%s\n", i18n.T("config_field_auto_switch"), strconv.FormatBool(cfg.AutoSwitchRules.Enabled))
			fmt.Fprintf(w, "%s\t%.1f\n", i18n.T("config_field_threshold"), cfg.AutoSwitchRules.MaxUsagePercent)
			fmt.Fprintf(w, "%s\t%s\n", i18n.T("config_field_auto_update"), strconv.FormatBool(cfg.AutoUpdateOnSwitchExpired))
			fmt.Fprintf(w, "%s\t%d\n", i18n.T("config_field_accounts"), len(cfg.Accounts))
			fmt.Fprintf(w, "%s\t%s\n", i18n.T("config_field_active"), active)
			return w.Flush()
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := bootstrap()
			key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(args[0]), "_", "-"))
			value := args[1]
			switch key {
			case "language", "lang":
				normalized, ok := i18n.Normalize(value)
				if !ok {
					return errors.New(i18n.T("invalid_language"))
				}
				svc.Cfg.Language = normalized
			case "sort-mode", "sort":
				switch strings.ToLower(strings.ReplaceAll(value, "_", "-")) {
				case "none":
					svc.Cfg.SortMode = models.SortNone
				case "reset-time":
					svc.Cfg.SortMode = models.SortResetTime
				case "window-remaining":
					svc.Cfg.SortMode = models.SortWindowRemaining
				default:
					return errors.New(i18n.T("invalid_sort_mode"))
				}
			case "auto-switch":
				v, err := strconv.ParseBool(value)
				if err != nil {
					return err
				}
				svc.Cfg.AutoSwitchRules.Enabled = v
			case "threshold", "max-usage-percent":
				v, err := strconv.ParseFloat(value, 32)
				if err != nil || v < 50 || v > 100 {
					return errors.New(i18n.T("invalid_threshold"))
				}
				svc.Cfg.AutoSwitchRules.MaxUsagePercent = float32(v)
			case "auto-update-on-switch-expired":
				v, err := strconv.ParseBool(value)
				if err != nil {
					return err
				}
				svc.Cfg.AutoUpdateOnSwitchExpired = v
			case "data-dir":
				if strings.TrimSpace(value) == "" {
					return errors.New(i18n.T("invalid_data_dir"))
				}
				if err := store.SaveLocalConfig(value); err != nil {
					return err
				}
				fmt.Println(i18n.T("config_saved"))
				return nil
			case "atomcode-dir":
				svc.Cfg.SetAtomcodeDir(strings.TrimSpace(value))
			default:
				return errors.New(i18n.T("config_unknown_key", args[0]))
			}
			if err := svc.Save(); err != nil {
				return err
			}
			fmt.Println(i18n.T("config_saved"))
			return nil
		},
	}
}
