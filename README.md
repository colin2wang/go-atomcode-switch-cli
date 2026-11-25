# AtomCode Switch CLI

![Go](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go&logoColor=white) ![Task](https://img.shields.io/badge/Task-3.28%2B-3E77B0)

A Go CLI for managing and switching between multiple AtomCode accounts. It imports credentials from AtomCode's local `auth.toml`, keeps account snapshots (token, plan, usage, reset time) in YAML, and writes the selected account back to `auth.toml`. It works both as a headless command-line tool and as an interactive TTY menu.

## Features

- List, import, update, delete, export, and switch AtomCode accounts.
- Interactive menu with arrow-key navigation on any TTY terminal.
- Refresh account information from `atomcode login` output, or paste/provide that output offline.
- Threshold-based automatic switching with a 5-point hysteresis to avoid flapping.
- Scheduler-friendly `daemon --once` mode for cron or similar schedulers.
- Cross-platform builds for Linux, macOS, and Windows.

## Requirements

- Go 1.27.3 and [Task](https://taskfile.dev) 3.28+ for building from source.
- The `atomcode` executable on `PATH` for the default `update` behavior. You can also use `--text`, `--input-file`, or `--stdin` to supply `/login` output without running it.

## Build and install

```bash
task install                      # go install into GOPATH/bin
task build                        # build for the current OS/architecture
task build PLATFORM=linux/arm64   # build for one target
task build ALL=true               # build all targets in the platform matrix
task build-all                    # same as ALL=true
```

Build artifacts are written to `dist/atomcode-switch-cli-<os>-<arch>` (with `.exe` on Windows). The version is injected at build time with `-X main.version=<version>`; the default is `dev`.

Useful checks:

```bash
task fmt       # gofmt
task vet       # go vet
task test      # go test -count=1 ./...
task check     # fmt-check + vet + test
```

Release helpers:

```bash
task package   # .tar.gz for Linux/macOS, .zip for Windows
task checksum  # SHA256SUMS (or SHA256SUMS.txt on Windows)
```

Supported build targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`.

## Quick start

```bash
# Import the current ~/.atomcode/auth.toml (or the configured AtomCode directory).
atomcode-switch-cli import

# Show all managed accounts.
atomcode-switch-cli list

# Switch by 1-based index, account ID, display name, or username.
atomcode-switch-cli switch 2
atomcode-switch-cli switch alice

# Refresh usage/plan/reset-time from atomcode login.
atomcode-switch-cli update

# Launch the interactive menu (also: m, ui).
atomcode-switch-cli menu
```

Running `atomcode-switch-cli` with no arguments opens the interactive menu when stdin is a TTY. With redirected stdin, it prints help instead.

## Commands

| Command | Aliases | Description |
| --- | --- | --- |
| `atomcode-switch-cli` | — | Open the menu on a TTY; otherwise show help. |
| `menu` | `m`, `ui` | Open the interactive menu. Requires a TTY. |
| `list` | `ls` | List accounts in a table. |
| `import` | — | Import or sync the current `auth.toml`. |
| `update` | — | Run `atomcode login`, parse its `/login` output, and update the active account. |
| `switch <index\|id\|name>` | — | Switch the active account and write `auth.toml`. |
| `delete <account>` | — | Delete an account. Prompts for confirmation unless `-y` is given. |
| `export` | — | Export accounts to YAML. Default output: `atomcode-accounts.yaml`. |
| `import-file <yaml>` | — | Import accounts from an exported YAML file. |
| `config show` | — | Show effective configuration. |
| `config set <key> <value>` | — | Update a configuration value. |
| `daemon` | — | Check auto-switch rules periodically. |

The global flag `--data-dir <dir>` overrides the data directory for any command. Use `--version` to print the injected version.

### Command options

```text
switch <index|id|name> [--update]
    --update                  Run update if the target account is expired.

update [--timeout DURATION] [--text TEXT] [--input-file FILE] [--stdin]
    --timeout DURATION        Fetch timeout for atomcode login (default 10s).
    --text TEXT               Use supplied /login output instead of running atomcode login.
    --input-file FILE         Read /login output from a file.
    --stdin                   Read /login output from stdin.

delete <account> [--yes]
    -y, --yes                 Skip the confirmation prompt.

export [--output FILE]
    -o, --output FILE         Output file path (default atomcode-accounts.yaml).

daemon [--interval DURATION] [--once]
    --interval DURATION       Check interval (default 30s).
    --once                    Run one check and exit, for cron.
```

Examples:

```bash
atomcode-switch-cli update --timeout 30s
atomcode-switch-cli update --input-file login-output.txt
cat login-output.txt | atomcode-switch-cli update --stdin
atomcode-switch-cli export -o backup.yaml
atomcode-switch-cli import-file backup.yaml
atomcode-switch-cli delete alice -y
atomcode-switch-cli daemon --once
atomcode-switch-cli --data-dir /srv/atomcode-switch list
```

`import-file` replaces accounts, the active account, and auto-switch rules, while preserving local settings such as language and sort mode.

## Configuration

The data directory is resolved in this order:

1. `--data-dir` flag
2. `ATOMCODE_SWITCH_DATA_DIR` environment variable
3. `custom_data_dir` in `config.yaml` next to the executable
4. `~/.atomcode-switch` (`%USERPROFILE%\.atomcode-switch` on Windows)

The account database is `atomcode-accounts.yaml` inside the data directory. The executable-side `config.yaml` only stores `custom_data_dir`; it is written by `config set data-dir <path>`.

`config set` accepts these keys:

| Key | Values |
| --- | --- |
| `language`, `lang` | `zh_cn`, `en_us` |
| `sort-mode`, `sort` | `none`, `reset-time`, `window-remaining` |
| `auto-switch` | `true`, `false` |
| `threshold`, `max-usage-percent` | number from 50 to 100; default `95` |
| `auto-update-on-switch-expired` | `true`, `false` |
| `data-dir` | path; writes `config.yaml` next to the executable |
| `atomcode-dir` | path to the AtomCode directory; an empty value clears the override |

```bash
atomcode-switch-cli config show
atomcode-switch-cli config set language en_us
atomcode-switch-cli config set auto-switch true
atomcode-switch-cli config set threshold 90
atomcode-switch-cli config set sort-mode reset-time
atomcode-switch-cli config set data-dir /srv/atomcode-switch
```

By default, AtomCode credentials live at `~/.atomcode/auth.toml`. `config set atomcode-dir <path>` overrides that directory. When switching, the tool also writes a snapshot named `auth.toml.<username>`.

### Automatic switching

Auto-switching is disabled by default. When enabled, the tool switches only if:

- the active account's usage is at or above the threshold;
- another account is valid, marked fresh, and below the threshold; and
- that account's usage is at least 5 percentage points lower than the active account.

This 5-point hysteresis prevents rapid back-and-forth switching near the threshold. Use `daemon` to run the check continuously, or `daemon --once` from cron:

```cron
*/5 * * * * /usr/local/bin/atomcode-switch-cli daemon --once
```

## Security

`atomcode-accounts.yaml`, exported YAML files, and `auth.toml.<username>` snapshots contain plaintext tokens. Treat them as secrets:

- Do not commit them to version control or paste them into logs/issues.
- Store exported files in a protected location and delete them when no longer needed.
- Restrict filesystem permissions for the data directory; the tool writes files atomically with restrictive permissions.

## Platform notes

- The interactive menu requires a TTY. On Unix terminals, raw mode keeps output line endings correct after entering and leaving line-input mode.
- On Windows, the terminal uses VT input/output and the home directory falls back to `USERPROFILE` before `HOME`.
- The default `update` command starts `atomcode login`; use one of the offline input options if that executable is unavailable.

## Project layout

- `cmd/atomcode-switch-cli`: program entry point and version injection.
- `internal/cli`: Cobra commands and interactive menu.
- `internal/ops`: business logic for account selection, switching, import/update, and auto-switch rules.
- `internal/store`: configuration and account persistence.
- `internal/parser`, `internal/atomio`, `internal/fetch`: AtomCode credential parsing, `auth.toml` I/O, and login-output collection.
- `internal/i18n`: English and Chinese user-facing messages.

See `AGENTS.md` for development conventions and maintenance rules.
