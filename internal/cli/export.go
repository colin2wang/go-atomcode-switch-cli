package cli

import (
	"errors"
	"fmt"

	"go-atomcode-switch-cli/internal/i18n"

	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export accounts to YAML (default: atomcode-accounts.yaml)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := bootstrap()
			if output == "" {
				output = "atomcode-accounts.yaml"
			}
			if err := svc.Store.Export(output, svc.Cfg); err != nil {
				return errors.New(i18n.T("export_failed", err))
			}
			fmt.Println(i18n.T("exported", output))
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path")
	return cmd
}
