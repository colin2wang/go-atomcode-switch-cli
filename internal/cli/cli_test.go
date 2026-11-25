package cli

import (
	"io"
	"strings"
	"testing"

	"go-atomcode-switch-cli/internal/models"
	"go-atomcode-switch-cli/internal/ops"
	"go-atomcode-switch-cli/internal/store"

	"github.com/spf13/cobra"
)

func findCommand(root *cobra.Command, name string) *cobra.Command {
	for _, cmd := range root.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}

func TestNewRootRegistersCommands(t *testing.T) {
	flagDataDir = ""
	for _, name := range []string{
		"menu", "list", "switch", "import", "update", "delete", "export", "import-file", "config", "daemon",
	} {
		if findCommand(NewRoot(), name) == nil {
			t.Errorf("command %q not registered", name)
		}
	}
	menu := findCommand(NewRoot(), "menu")
	if menu == nil || len(menu.Aliases) != 2 || menu.Aliases[0] != "m" || menu.Aliases[1] != "ui" {
		t.Fatalf("unexpected menu aliases: %+v", menu)
	}
}

func TestRootPersistentDataDirFlag(t *testing.T) {
	root := NewRoot()
	if root.PersistentFlags().Lookup("data-dir") == nil {
		t.Fatal("data-dir flag not registered")
	}
}

func TestCommandArgumentValidation(t *testing.T) {
	root := NewRoot()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"switch"})
	if err := root.Execute(); err == nil {
		t.Fatal("switch without an argument should fail")
	}
}

func TestCommandFlags(t *testing.T) {
	root := NewRoot()
	cases := map[string][]string{
		"switch": {"update"},
		"update": {"timeout", "text", "input-file", "stdin"},
		"delete": {"yes"},
		"export": {"output"},
		"daemon": {"interval", "once"},
	}
	for commandName, flags := range cases {
		cmd := findCommand(root, commandName)
		if cmd == nil {
			t.Fatalf("command %q not found", commandName)
		}
		for _, flag := range flags {
			if cmd.Flags().Lookup(flag) == nil && cmd.InheritedFlags().Lookup(flag) == nil {
				t.Errorf("command %q missing flag %q", commandName, flag)
			}
		}
	}
	if findCommand(root, "config").Run != nil {
		t.Fatal("config should have subcommands instead of a direct Run function")
	}
}

func TestMenuItemsShowsCountAndActive(t *testing.T) {
	svc := ops.New(models.DefaultConfig(), store.New(t.TempDir()))
	svc.Cfg.Accounts = []models.ManagedAccount{
		{
			ID:       "a",
			AuthData: models.AuthToml{User: models.User{Name: "Alice"}},
			IsValid:  true,
		},
	}
	svc.Cfg.SetActiveID("a")

	items := menuItems(svc)
	if len(items) != 10 {
		t.Fatalf("menu items = %d, want 10", len(items))
	}
	if !strings.Contains(items[0], "1") {
		t.Fatalf("list item does not show account count: %q", items[0])
	}
	if !strings.Contains(items[8], "Alice") {
		t.Fatalf("settings item does not show active account: %q", items[8])
	}
}

func TestOnOffUsesDistinctLabels(t *testing.T) {
	if onOff(true) == onOff(false) {
		t.Fatal("onOff returned the same label for true and false")
	}
}
