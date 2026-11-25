package cli

import (
	"errors"
	"fmt"

	"go-atomcode-switch-cli/internal/i18n"

	"github.com/spf13/cobra"
)

func newImportFileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import-file <yaml>",
		Short: "Import accounts from a YAML file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := bootstrap()
			n, err := svc.ImportFile(args[0])
			if err != nil {
				return errors.New(i18n.T("import_file_bad", err))
			}
			fmt.Println(i18n.T("imported", n))
			return nil
		},
	}
}
