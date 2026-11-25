package cli

import (
	"fmt"
	"os"

	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/ops"
	"go-atomcode-switch-cli/internal/store"

	"github.com/spf13/cobra"
)

var (
	flagDataDir string
	appVersion  = "dev"
)

// SetVersion 设置由 ldflags 注入的版本号。
func SetVersion(v string) {
	if v != "" {
		appVersion = v
	}
}

// NewRoot 创建根命令并注册所有子命令。
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "atomcode-switch-cli",
		Short:         "Manage and switch multiple AtomCode accounts (headless CLI)",
		Version:       appVersion,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if stdinIsTTY() {
				return runMenu()
			}
			return cmd.Help()
		},
	}
	root.PersistentFlags().StringVar(&flagDataDir, "data-dir", "", "custom data directory (default ~/.atomcode-switch)")
	root.AddCommand(
		newMenuCmd(),
		newListCmd(),
		newSwitchCmd(),
		newImportCmd(),
		newUpdateCmd(),
		newDeleteCmd(),
		newExportCmd(),
		newImportFileCmd(),
		newConfigCmd(),
		newDaemonCmd(),
	)
	return root
}

// Execute 解析命令行参数并执行。
func Execute() error {
	return NewRoot().Execute()
}

func bootstrap() *ops.Service {
	dataDir := store.ResolveDataDir(flagDataDir)
	st := store.New(dataDir)
	cfg, err := st.Load()
	i18n.Init(cfg.Language)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("config_load_failed", err))
	}
	return ops.New(cfg, st)
}
