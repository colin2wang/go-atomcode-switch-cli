package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"go-atomcode-switch-cli/internal/fetch"
	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/ops"

	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var timeout time.Duration
	var text string
	var inputFile string
	var fromStdin bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Run atomcode login, parse output and update account info",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(bootstrap(), timeout, text, inputFile, fromStdin)
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "fetch timeout")
	cmd.Flags().StringVar(&text, "text", "", "use provided /login output instead of running atomcode login")
	cmd.Flags().StringVar(&inputFile, "input-file", "", "read /login output from a file")
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read /login output from stdin")
	return cmd
}

func runUpdate(svc *ops.Service, timeout time.Duration, text, inputFile string, fromStdin bool) error {
	var out string
	if text != "" || inputFile != "" || fromStdin {
		var err error
		out, err = readLoginText(text, inputFile, fromStdin)
		if err != nil {
			return err
		}
	} else {
		fmt.Fprintln(os.Stderr, i18n.T("fetching"))
		var err error
		out, err = fetch.FetchLoginOutput(context.Background(), timeout)
		if err != nil {
			switch {
			case errors.Is(err, fetch.ErrTimeout):
				return errors.New(i18n.T("fetch_timeout"))
			case errors.Is(err, fetch.ErrNoOutput):
				return errors.New(i18n.T("fetch_no_output"))
			default:
				return errors.New(i18n.T("fetch_spawn_failed", err))
			}
		}
	}

	name, err := svc.ApplyLoginOutput(out)
	if err != nil {
		switch {
		case errors.Is(err, ops.ErrUserUnmatched):
			loginName := strings.TrimSpace(strings.TrimPrefix(err.Error(), ops.ErrUserUnmatched.Error()+": "))
			return errors.New(i18n.T("user_unmatched", loginName))
		case errors.Is(err, ops.ErrNoFields):
			return errors.New(i18n.T("no_fields"))
		case errors.Is(err, ops.ErrNoActive):
			return errors.New(i18n.T("no_active"))
		case errors.Is(err, ops.ErrActiveNotFound):
			return errors.New(i18n.T("active_not_found"))
		default:
			return err
		}
	}
	if _, _, err := svc.ImportCurrentAuth(); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("auth_not_found"))
	}
	fmt.Println(i18n.T("updated_from_paste", name))
	return nil
}

func readLoginText(text, path string, fromStdin bool) (string, error) {
	if text != "" {
		return text, nil
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", errors.New(i18n.T("import_file_bad", err))
		}
		return string(data), nil
	}
	if fromStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return "", nil
}
