package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/ops"

	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <account>",
		Short: "Delete an account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := bootstrap()
			acc, err := svc.Resolve(args[0])
			if err != nil {
				return err
			}
			if !yes {
				fmt.Printf("%s [y/N] ", i18n.T("delete_confirm", acc.AuthData.User.Name))
				answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
				if err != nil && answer == "" {
					fmt.Println(i18n.T("delete_canceled"))
					return nil
				}
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "y" && answer != "yes" {
					fmt.Println(i18n.T("delete_canceled"))
					return nil
				}
			}
			if err := svc.Delete(acc.ID); err != nil {
				if errors.Is(err, ops.ErrNotFound) {
					return errors.New(i18n.T("account_not_found", args[0]))
				}
				return err
			}
			fmt.Println(i18n.T("account_deleted"))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}
