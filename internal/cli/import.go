package cli

import (
	"errors"
	"fmt"

	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/ops"

	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import",
		Short: "Import or sync the current auth.toml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := bootstrap()
			isNew, name, err := svc.ImportCurrentAuth()
			if err != nil {
				if errors.Is(err, ops.ErrAuthNotFound) {
					return errors.New(i18n.T("auth_not_found"))
				}
				return err
			}
			if isNew {
				fmt.Println(i18n.T("imported_hint", name))
			} else {
				fmt.Println(i18n.T("imported_existing", name))
			}
			return nil
		},
	}
}
