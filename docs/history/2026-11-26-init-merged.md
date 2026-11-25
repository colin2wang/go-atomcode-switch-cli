# AtomCode 多账号管理 CLI — 设计文档（合并版）

> **来源与说明**：本文档由两份迭代设计文档合并而成——
> - `2026-11-26-init.md`：**初版重写方案**（Rust GUI → Go CLI 的完整设计：需求映射、技术选型、目录结构、CLI 命令、核心代码、构建部署、语义对照）；
> - `2026-11-26-init-v2.md`：**增量方案**（交互菜单模式 + `taskfile.yml` 优化）。
>
> 合并时以 `go-atomcode-switch-cli` 仓库的**当前实现**为准，对齐了以下两处文档间的项目级差异：
> - **命名**：模块名 / 二进制名统一为 `go-atomcode-switch-cli`（初版写作 `atomcode-switch`）；
> - **构建体系**：以 `taskfile.yml` 取代初版的 `Makefile`；
> - **关键 API**：`runUpdate` 五参签名（含 `--text/--input-file/--stdin`）、`models.ActiveID/SetActiveID`、`i18n.Init/Normalize/Current`、`ops.SyncCurrentAuthExisting/ImportFile`、raw 模式按平台拆分（`tui_windows.go` / `tui_raw_darwin.go` / `tui_raw_linux.go` / `tui_raw_unix.go` / `tui_other.go`）。
>
> 个别代码细节（如 i18n 完整 key 集合、`ops` 方法体）已对齐仓库源码；若与仓库最新代码有出入，**以仓库源码为准**。

---

## 一、项目背景与目标

原项目是一个用 Rust + egui 编写的 **AtomCode 多账号管理 GUI 工具**：在本地存储多个 AtomCode 账号的凭据（`atomcode-accounts.yaml`），通过切换写入 `~/.atomcode/auth.toml` 来"换号"，并能解析 `atomcode login`（`/login`）输出刷新用量/到期信息，在用量超阈值时自动切换到更空闲的账号。

本次目标是把它**重写为 headless 的 Go CLI**：

- 在 **Linux 服务器 / SSH** 场景下直接可用，不依赖 GUI 与显示环境；
- 可被**脚本化**驱动（`cron` / `systemd` / CI 管道），非 TTY 下行为确定（打印帮助而非进入交互）；
- **完整保留 GUI 版的核心业务语义**（多账号存储、账号切换、自动切换、`/login` 输出解析、中英 i18n、防串号、事务性落盘），同时**删掉 GUI 专属的复杂度**（视口/主题/消息气泡/延迟关窗/后台竞态防护）。
- 在 v2 中进一步补上**交互菜单模式**（TTY 下无参数即进入方向键菜单），让"裸跑一个二进制名"在服务器终端里也能获得接近 GUI 的操作体验；并把构建体系从 `Makefile` 迁移到数据驱动的 `taskfile.yml`。

**架构边界（两版一致）**：`internal/ops` 是唯一承载业务规则的层；命令层（`internal/cli`）与菜单层都只是 `ops.Service` 之上的**薄壳**，不各自新增业务逻辑。

---

## 二、需求分析：GUI → CLI 的功能映射

先梳理原项目的**核心功能**（与 UI 无关的部分），这些是重写的主体：

| 原功能 | Rust 实现 | Go CLI 版对应 |
|---|---|---|
| 多账号存储（atomcode-accounts.yaml） | serde_yaml + 结构体级 default | yaml.v3 + `Normalize()` 补默认值 |
| 切换账号（写 `~/.atomcode/auth.toml`） | 原子写（无 .bak）+ 用户名快照 | 同语义保留 |
| 导入/同步当前 auth.toml | import_current_auth | `import` 命令 + 菜单「同步当前 auth.toml」 |
| `/login` 输出解析（中英文、零字段不落盘、事务性应用） | login_parser 纯函数 | 正则纯函数，语义 1:1 |
| 统一时间解析（新 `YYYY-MM-DD HH:MM` / 旧 `HH:MM` 顺延） | parse_reset_datetime | timeutil 包，语义 1:1 |
| 自动切换（阈值 + 5% 滞回 + 仅 Fresh 参与） | check_auto_switch（每帧 30s） | `daemon`（常驻）或 `daemon --once`（cron） |
| 更新账号信息（跑 atomcode login → 解析） | 后台线程 + generation 防竞态 + 窗口状态机 | **前台同步执行**，状态机天然消失 |
| 导出/导入账号 | atomic_write_text | `export` / `import-file` |
| 解析失败备份 .bak 回退默认 | load_config | 同语义保留 |
| i18n 中英双语 | t0~t4 + ALL_KEYS 对齐测试 | 轻量 message map + key 对齐测试 |
| 登录用户≠激活账号时防串号 | status_login_user_unmatched | 同语义保留 |
| 账号选择/弹窗/设置（GUI 交互） | egui 视口 + 状态机 | **`menu` 子命令 + 无参数默认菜单**（v2 新增） |

**CLI 化后可以删掉的复杂度**：egui 视口/主题/消息气泡/延迟关窗/`fetch_generation` 竞态防护——update 在 CLI 里是前台同步流程（fetch → parse → apply → save），没有"关窗后后台结果落盘"的问题。

---

## 三、技术选型

| 组件 | 选择 | 理由 |
|---|---|---|
| **CLI 框架** | **cobra** (spf13/cobra) | 子命令/flag/自动 help/completion 事实标准；本项目天然是多子命令形态（list/switch/update/daemon/menu） |
| 配置解析 | yaml.v3（不用 viper） | 配置 schema 固定（单一 accounts.yaml + local config.yaml），viper 的多来源合并/热更新收益不大，直接 yaml.v3 更可控；日后需要再引入 |
| auth.toml | BurntSushi/toml | 与 AtomCode CLI 的 TOML 格式互操作，支持 omitempty（对应"不写出空 avatar_url"） |
| 时间 | 标准库 `time` | chrono 的能力足够覆盖 |
| 表格输出 | 标准库 `text/tabwriter` | 零依赖；要彩色可另加 fatih/color（可选） |
| 日志 | `log/slog` | 结构化日志，适合 server 常驻进程 |
| **交互菜单**（v2） | **`golang.org/x/term`**（+ `golang.org/x/sys` 随之转 direct） | 自研 ~150 行轻量方向键选择器，与项目"轻量依赖"哲学一致（Rust 版用 tinyfiledialogs 替代 rfd 同源思路），避免 bubbletea 的 20~30 个间接依赖 |

---

## 四、目录结构

```
go-atomcode-switch-cli/
├── cmd/atomcode-switch-cli/
│   └── main.go                 # 唯一入口：注入版本 + cli.Execute()
├── internal/
│   ├── cli/                    # cobra 命令层 + 交互菜单（薄壳，无业务规则）
│   │   ├── cli.go              # root 命令、Execute、bootstrap（含 i18n.Init）
│   │   ├── menu.go             # 交互菜单主循环 + 全部动作
│   │   ├── tui.go              # raw 模式 / 方向键选择器 / 渲染（跨平台公共部分）
│   │   ├── tui_windows.go      # Windows VT 输入/输出位（vtInput / vtProcessing）
│   │   ├── tui_raw_darwin.go   # darwin raw 模式
│   │   ├── tui_raw_linux.go    # linux raw 模式
│   │   ├── tui_raw_unix.go     # 其它 unix raw 模式
│   │   ├── tui_other.go        # 兜底（无 raw 依赖时）
│   │   ├── list.go             # list 命令 + printAccounts（CLI 与菜单共用）
│   │   ├── switch.go           # switch
│   │   ├── import.go           # import
│   │   ├── update.go           # update（--timeout / --text / --input-file / --stdin）
│   │   ├── delete.go           # delete
│   │   ├── export.go           # export
│   │   ├── import_file.go      # import-file
│   │   ├── config.go           # config show | set
│   │   └── daemon.go           # daemon（--interval / --once）
│   ├── models/                 # AppConfig / ManagedAccount / AuthToml + 默认值兼容 + ActiveID/SetActiveID
│   ├── timeutil/               # parse_reset_datetime 单一解析入口
│   ├── parser/                 # /login 输出解析纯函数
│   ├── store/                  # 原子写、配置加载/保存、旧数据迁移、导出导入
│   ├── atomio/                 # ~/.atomcode/auth.toml 读写 + 用户名快照
│   ├── fetch/                  # 执行 atomcode login 捕获输出（管道背压/超时/早退）
│   ├── ops/                    # 业务：切换/导入/删除/自动切换/解析落盘（唯一业务层）
│   └── i18n/                   # 中英双语（Init/Normalize/Current + key 对齐测试）
├── taskfile.yml                # 构建与发布入口（数据驱动平台矩阵）
└── go.mod
```

---

## 五、CLI 命令设计

```text
atomcode-switch-cli                              # 无参数：TTY → 交互菜单；非 TTY → 帮助
atomcode-switch-cli menu  [m | ui]               # 显式进入交互菜单（别名 m / ui）
atomcode-switch-cli list  [ls]                   # 列出账号（含状态/倒计时，替代卡片视图）
atomcode-switch-cli switch <index|id|name>       # 切换账号（写 auth.toml），--update 过期时自动更新
atomcode-switch-cli import                       # 导入/同步当前 auth.toml（原「同步登录信息」）
atomcode-switch-cli update [--timeout 10s]       # 跑 atomcode login → 解析 → 更新
atomcode-switch-cli update [--text ... | --input-file <f> | --stdin]
atomcode-switch-cli delete <account> [--yes]     # 删除账号（active 被删时自动补位）
atomcode-switch-cli export [-o file]             # 导出（默认名 atomcode-accounts.yaml）
atomcode-switch-cli import-file <yaml>           # 从 YAML 导入替换账号
atomcode-switch-cli config show | set ...        # 阈值/开关/排序/语言
atomcode-switch-cli daemon [--interval 30s]      # 常驻自动切换（systemd）
atomcode-switch-cli daemon --once                # 单次检查（cron 场景）
```

- **根命令无参数**（v2）：stdin 是 TTY 时直接进入交互菜单（服务器上敲一个二进制名即可）；非 TTY（脚本/CI/管道）保持旧行为显示帮助，向后兼容。
- **`menu` 子命令**：别名 `m` / `ui`；要求 TTY，非 TTY 下返回 `menu_requires_tty`。
- **`update` 的输入来源**（对齐实现）：默认执行 `atomcode login`；也可用 `--text` 直接喂 `/login` 输出、`--input-file <f>` 从文件读、`--stdin` 从标准输入读，便于离线/测试。

---

## 六、交互菜单模式（v2）

### 6.1 选型对比

| 方案 | 依赖成本 | 优点 | 缺点 |
|---|---|---|---|
| **charmbracelet/bubbletea** | +20~30 个间接依赖 | 完整 TUI 框架、跨平台终端处理最完善 | 依赖重，与本项目刻意轻量化的风格（Rust 版用 tinyfiledialogs 替代 rfd）相悖；对"菜单选账号"这种需求是杀鸡用牛刀 |
| AlecAivazis/survey、manifoldco/promptui | 中等 | 组件现成 | **均已停止维护**，不建议新项目引入 |
| **自研 + golang.org/x/term（推荐）** | **+1 个直接依赖**（x/sys 随之转 direct） | 依赖极小；raw 模式 + ANSI 转义即可实现方向键菜单；SSH/server 场景完全够用；Go 官方维护 | 需自己处理各平台 raw/VT 位（约 20 行，下面已给出） |

**决策：自研轻量组件**。如果未来需要复杂表单/实时刷新视图，再整体替换为 bubbletea——菜单逻辑被隔离在 `tui.go`，替换成本可控。

### 6.2 交互设计

```
$ atomcode-switch-cli          # 无参数 = 直接进菜单（TTY 下）
$ atomcode-switch-cli menu     # 显式进入（别名 m / ui）
┌────────────────────────────────────────┐
│ AtomCode 账号管理                        │
│                                        │
│ > 查看账号（共 3 个）                     │
│   切换账号                               │
│   更新账号信息（执行 /login）              │
│   同步当前 auth.toml                     │
│   立即检查自动切换（当前: 开, 阈值 95%）    │
│   删除账号                               │
│   导出账号                               │
│   从文件导入账号                          │
│   设置（激活: 张三）                      │
│   退出                                  │
│                                        │
│ ↑/↓ 选择, Enter 确认, q 取消             │
└────────────────────────────────────────┘
```

- **按键**：`↑/↓` 移动、`Enter` 确认、`q`/`Esc`/`Ctrl+C` 返回或退出；
- **操作后**：显示结果 → "按回车返回…" → 回主菜单（等价于 GUI 卡片 → 弹窗 → 返回）；
- **每次进入主菜单前重新 `bootstrap()`**：重读磁盘配置，外部修改（手动编辑 YAML、另一个终端切换）能即时反映——对应 GUI 每帧刷新的语义；
- **设置子菜单**：语言切换/排序方式/自动开关用单键 toggle（无输入成本），阈值用数值输入（临时切回规范模式）。

### 6.3 核心代码

**go.mod 变更**（x/sys 随之转 direct）：

```text
require golang.org/x/term v0.27.0
require golang.org/x/sys   v0.28.0   // go get golang.org/x/term 后 go mod tidy
```

**`internal/cli/tui.go`** —— 终端助手（raw 模式 / 选择器 / 渲染，跨平台公共部分）：

```go
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"go-atomcode-switch-cli/internal/i18n"

	"golang.org/x/term"
)

const (
	ansiClear = "\033[2J\033[H"
	ansiBold  = "\033[1m"
	ansiDim   = "\033[2m"
	ansiReset = "\033[0m"
)

type menuKey int

const (
	keyUp menuKey = iota
	keyDown
	keyEnter
	keyQuit
	keyOther
)

type tty struct {
	fd     int
	isTTY  bool
	origin *term.State
}

func stdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func newTTY() (*tty, error) {
	fd := int(os.Stdin.Fd())
	t := &tty{fd: fd, isTTY: stdinIsTTY()}
	if t.isTTY {
		st, err := term.MakeRaw(fd)
		if err != nil {
			return nil, err
		}
		t.origin = st
		enableRawModeExtras()
	}
	return t, nil
}

func (t *tty) restore() {
	if t.isTTY && t.origin != nil {
		_ = term.Restore(t.fd, t.origin)
		t.origin = nil
	}
}

// canonical 暂时恢复规范模式执行 fn（需要行编辑的输入），随后重新进入 raw 模式。
func (t *tty) canonical(fn func() error) error {
	t.restore()
	err := fn()
	if t.isTTY {
		st, rawErr := term.MakeRaw(t.fd)
		if rawErr != nil {
			return rawErr
		}
		t.origin = st
		enableRawModeExtras()
	}
	return err
}

func (t *tty) readKey() menuKey {
	buf := make([]byte, 3)
	n, err := os.Stdin.Read(buf)
	if err != nil || n == 0 {
		return keyQuit
	}
	switch {
	case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
		return keyEnter
	case n == 1 && (buf[0] == 'q' || buf[0] == 'Q' || buf[0] == 3 || buf[0] == 27):
		return keyQuit
	case n >= 3 && buf[0] == 0x1b && buf[1] == '[' && buf[2] == 'A':
		return keyUp
	case n >= 3 && buf[0] == 0x1b && buf[1] == '[' && buf[2] == 'B':
		return keyDown
	default:
		return keyOther
	}
}

// choose 渲染方向键选择器；返回选中下标，q/Esc/Ctrl+C 取消时返回 -1。
func (t *tty) choose(title string, items []string, footer string) int {
	if len(items) == 0 {
		return -1
	}
	cursor := 0
	for {
		fmt.Print(ansiClear)
		fmt.Println(ansiBold + title + ansiReset)
		fmt.Println()
		for i, item := range items {
			if i == cursor {
				fmt.Printf("%s> %s%s\n", ansiBold, item, ansiReset)
			} else {
				fmt.Printf("  %s\n", item)
			}
		}
		fmt.Println()
		if footer != "" {
			fmt.Println(ansiDim + footer + ansiReset)
		}
		fmt.Println(ansiDim + i18n.T("menu_choose_hint") + ansiReset)
		switch t.readKey() {
		case keyUp:
			if cursor > 0 {
				cursor--
			}
		case keyDown:
			if cursor < len(items)-1 {
				cursor++
			}
		case keyEnter:
			return cursor
		case keyQuit:
			return -1
		}
	}
}

func (t *tty) confirm() bool {
	return t.choose(i18n.T("menu_confirm_title"),
		[]string{i18n.T("menu_yes"), i18n.T("menu_no")}, "") == 0
}

func (t *tty) pause() {
	fmt.Println()
	fmt.Println(ansiDim + i18n.T("menu_press_enter") + ansiReset)
	for {
		switch t.readKey() {
		case keyEnter, keyQuit:
			return
		}
	}
}

// readLine 临时切回规范模式读一行（有行编辑/粘贴支持）；defaultVal 为空输入时的默认值。
func (t *tty) readLine(prompt, defaultVal string) string {
	var line string
	_ = t.canonical(func() error {
		if defaultVal != "" {
			fmt.Printf("%s [%s]: ", prompt, defaultVal)
		} else {
			fmt.Print(prompt)
		}
		s, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		line = strings.TrimSpace(s)
		if line == "" {
			line = defaultVal
		}
		return nil
	})
	fmt.Println()
	return line
}
```

**`internal/cli/tui_windows.go` / `tui_raw_*.go` / `tui_other.go`** —— 各平台 raw/VT 位（build tag 隔离）：

> Windows 控制台默认不开 VT：**不开输入位时方向键读不到 ESC 序列，不开输出位时 ANSI 清屏无效**，必须显式补上；Linux/Darwin 下 `term.MakeRaw` 会清掉输出标志，需重新打开 `OPOST|ONLCR` 才能让换行与 ANSI 正常。`enableRawModeExtras()` 是跨平台钩子，按平台各有一份实现。

```go
//go:build windows
package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

const (
	vtProcessing = 0x0004 // 输出：支持 ANSI 转义序列
	vtInput      = 0x0200 // 输入：方向键以 ESC [ A/B 序列返回
)

func enableRawModeExtras() {
	var mode uint32
	in := windows.Handle(os.Stdin.Fd())
	if windows.GetConsoleMode(in, &mode) == nil {
		_ = windows.SetConsoleMode(in, mode|vtInput)
	}
	out := windows.Handle(os.Stdout.Fd())
	if windows.GetConsoleMode(out, &mode) == nil {
		_ = windows.SetConsoleMode(out, mode|vtProcessing)
	}
}
```

```go
//go:build linux
package cli

import "golang.org/x/sys/unix"

const (
	rawTermiosGet = unix.TCGETS
	rawTermiosSet = unix.TCSETS
)
```

```go
//go:build darwin
package cli

import "golang.org/x/sys/unix"

const (
	rawTermiosGet = unix.TIOCGETA
	rawTermiosSet = unix.TIOCSETA
)
```

```go
//go:build linux || darwin
package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

func enableRawModeExtras() {
	fd := int(os.Stdin.Fd())
	termios, err := unix.IoctlGetTermios(fd, rawTermiosGet)
	if err != nil {
		return
	}
	termios.Oflag |= unix.OPOST | unix.ONLCR
	_ = unix.IoctlSetTermios(fd, rawTermiosSet, termios)
}
```

```go
//go:build !windows && !linux && !darwin
package cli

// 其它平台：终端原生支持 ANSI，无需额外处理。
func enableRawModeExtras() {}
```

**`internal/cli/menu.go`** —— 命令 + 主循环 + 全部动作（复用 `ops` 层，与子命令同一套业务）：

```go
package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/models"
	"go-atomcode-switch-cli/internal/ops"
	"go-atomcode-switch-cli/internal/timeutil"

	"github.com/spf13/cobra"
)

func newMenuCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "menu",
		Aliases: []string{"m", "ui"},
		Short:   "Interactive menu mode (arrow keys + Enter)",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMenu()
		},
	}
}

func runMenu() error {
	if !stdinIsTTY() {
		return errors.New(i18n.T("menu_requires_tty"))
	}
	t, err := newTTY()
	if err != nil {
		return errors.New(i18n.T("menu_requires_tty"))
	}
	defer t.restore()

	for {
		svc := bootstrap() // 每轮重读磁盘配置，反映外部变更（等价 GUI 每帧刷新）
		idx := t.choose(i18n.T("menu_title"), menuItems(svc), "")
		switch idx {
		case -1, 9: // 取消 / 退出
			return nil
		case 0:
			fmt.Print(ansiClear)
			printAccounts(os.Stdout, svc.Cfg)
			t.pause()
		case 1:
			menuSwitch(t, svc)
		case 2:
			menuUpdate(t, svc)
		case 3:
			menuSync(t, svc)
		case 4:
			menuAutoSwitch(t, svc)
		case 5:
			menuDelete(t, svc)
		case 6:
			menuExport(t, svc)
		case 7:
			menuImportFile(t, svc)
		case 8:
			menuSettings(t, svc)
		}
	}
}

// menuItems 生成主菜单文案（纯函数，可单测）。
func menuItems(svc *ops.Service) []string {
	active := "-"
	if acc := svc.Active(); acc != nil {
		active = acc.AuthData.User.Name
	}
	return []string{
		fmt.Sprintf(i18n.T("menu_item_list"), len(svc.Cfg.Accounts)),
		i18n.T("menu_item_switch"),
		i18n.T("menu_item_update"),
		i18n.T("menu_item_sync"),
		fmt.Sprintf(i18n.T("menu_item_auto_switch"),
			onOff(svc.Cfg.AutoSwitchRules.Enabled), svc.Cfg.AutoSwitchRules.MaxUsagePercent),
		i18n.T("menu_item_delete"),
		i18n.T("menu_item_export"),
		i18n.T("menu_item_import_file"),
		fmt.Sprintf(i18n.T("menu_item_settings"), active),
		i18n.T("menu_item_quit"),
	}
}

func onOff(b bool) string {
	if b {
		return i18n.T("menu_on")
	}
	return i18n.T("menu_off")
}

// ---------- 账号选择器 ----------
func chooseAccount(t *tty, svc *ops.Service, title string) int {
	if len(svc.Cfg.Accounts) == 0 {
		fmt.Println(i18n.T("list_empty"))
		t.pause()
		return -1
	}
	sortAccounts(svc.Cfg)
	now := time.Now()
	items := make([]string, len(svc.Cfg.Accounts))
	for i := range svc.Cfg.Accounts {
		a := &svc.Cfg.Accounts[i]
		mark := "  "
		if a.ID == svc.Cfg.ActiveID() {
			mark = "* "
		}
		items[i] = fmt.Sprintf("%s%-2d %-16s %-14s %3.0f%%  %-10s %s",
			mark, i+1, a.AuthData.User.Name, a.AuthData.User.Username,
			a.UsagePercent, resetCol(a, now), stateOf(a, now))
	}
	return t.choose(title, items, "")
}

// ---------- 各动作（复用 ops 层，与子命令完全同一套业务） ----------
func menuSwitch(t *tty, svc *ops.Service) {
	idx := chooseAccount(t, svc, i18n.T("menu_switch_title"))
	if idx < 0 {
		return
	}
	fmt.Print(ansiClear)
	acc := &svc.Cfg.Accounts[idx]
	if err := svc.Switch(acc.ID); err != nil {
		fmt.Println(i18n.T("switch_failed", err))
		t.pause()
		return
	}
	fmt.Println(i18n.T("switched", acc.AuthData.User.Name))
	// 原 GUI「切换过期账号自动打开更新窗口」→ 菜单里改为询问
	if timeutil.IsExpired(acc.ResetTime, time.Now()) {
		fmt.Println(i18n.T("switch_expired_hint", acc.AuthData.User.Name))
		if t.confirm() {
			if err := runUpdate(svc, 10*time.Second, "", "", false); err != nil {
				fmt.Println("error:", err)
			}
		}
	}
	t.pause()
}

func menuUpdate(t *tty, svc *ops.Service) {
	fmt.Print(ansiClear)
	// 直接复用 update 子命令的核心流程（fetch → parse → apply）
	if err := runUpdate(svc, 10*time.Second, "", "", false); err != nil {
		fmt.Println("error:", err)
	}
	t.pause()
}

func menuSync(t *tty, svc *ops.Service) {
	fmt.Print(ansiClear)
	isNew, name, err := svc.ImportCurrentAuth()
	switch {
	case errors.Is(err, ops.ErrAuthNotFound):
		fmt.Println(i18n.T("auth_not_found"))
	case err != nil:
		fmt.Println(err)
	case isNew:
		fmt.Println(i18n.T("imported_hint", name))
	default:
		fmt.Println(i18n.T("imported_existing", name))
	}
	t.pause()
}

func menuAutoSwitch(t *tty, svc *ops.Service) {
	fmt.Print(ansiClear)
	if name, switched := svc.CheckAutoSwitch(); switched {
		fmt.Println(i18n.T("auto_switched", name))
	} else {
		fmt.Println(i18n.T("auto_switch_none"))
	}
	t.pause()
}

func menuDelete(t *tty, svc *ops.Service) {
	idx := chooseAccount(t, svc, i18n.T("menu_delete_title"))
	if idx < 0 {
		return
	}
	fmt.Print(ansiClear)
	acc := &svc.Cfg.Accounts[idx]
	if !t.confirm() {
		fmt.Println(i18n.T("delete_canceled"))
		t.pause()
		return
	}
	if err := svc.Delete(acc.ID); err != nil {
		fmt.Println(i18n.T("account_not_found", acc.AuthData.User.Name))
	} else {
		fmt.Println(i18n.T("account_deleted"))
	}
	t.pause()
}

func menuExport(t *tty, svc *ops.Service) {
	fmt.Print(ansiClear)
	path := t.readLine(i18n.T("menu_export_prompt"), "atomcode-accounts.yaml")
	if err := svc.Store.Export(path, svc.Cfg); err != nil {
		fmt.Println(i18n.T("export_failed", err))
	} else {
		fmt.Println(i18n.T("exported", path))
	}
	t.pause()
}

func menuImportFile(t *tty, svc *ops.Service) {
	fmt.Print(ansiClear)
	path := t.readLine(i18n.T("menu_import_prompt"), "")
	if path == "" {
		t.pause()
		return
	}
	n, err := svc.ImportFile(path)
	if err != nil {
		fmt.Println(i18n.T("import_file_bad", err))
	} else {
		fmt.Println(i18n.T("imported", n))
	}
	t.pause()
}

// ---------- 设置子菜单 ----------
func menuSettings(t *tty, svc *ops.Service) {
	for {
		cfg := svc.Cfg
		items := []string{
			fmt.Sprintf(i18n.T("menu_settings_language"), cfg.Language),
			fmt.Sprintf(i18n.T("menu_settings_sort"), cfg.SortMode),
			fmt.Sprintf(i18n.T("menu_settings_auto_switch"), onOff(cfg.AutoSwitchRules.Enabled)),
			fmt.Sprintf(i18n.T("menu_settings_threshold"), cfg.AutoSwitchRules.MaxUsagePercent),
			i18n.T("menu_back"),
		}
		idx := t.choose(i18n.T("menu_settings_title"), items, "")
		switch idx {
		case -1, 4:
			return
		case 0: // 中英切换
			if cfg.Language == "en_us" {
				cfg.Language = "zh_cn"
			} else {
				cfg.Language = "en_us"
			}
			i18n.Init(cfg.Language)
			if err := svc.Save(); err != nil {
				fmt.Println(i18n.T("config_save_failed", err))
			}
		case 1: // 循环切换排序方式
			switch cfg.SortMode {
			case models.SortNone:
				cfg.SortMode = models.SortResetTime
			case models.SortResetTime:
				cfg.SortMode = models.SortWindowRemaining
			default:
				cfg.SortMode = models.SortNone
			}
			if err := svc.Save(); err != nil {
				fmt.Println(i18n.T("config_save_failed", err))
			}
		case 2: // 自动切换开关
			cfg.AutoSwitchRules.Enabled = !cfg.AutoSwitchRules.Enabled
			if err := svc.Save(); err != nil {
				fmt.Println(i18n.T("config_save_failed", err))
			}
		case 3: // 阈值
			menuSetThreshold(t, svc)
		}
	}
}

func menuSetThreshold(t *tty, svc *ops.Service) {
	for {
		raw := t.readLine(i18n.T("menu_threshold_prompt"), "")
		if raw == "" {
			return
		}
		v, err := strconv.ParseFloat(raw, 32)
		if err != nil || v < 50 || v > 100 {
			fmt.Println(i18n.T("invalid_threshold"))
			continue
		}
		svc.Cfg.AutoSwitchRules.MaxUsagePercent = float32(v)
		if err := svc.Save(); err != nil {
			fmt.Println(i18n.T("config_save_failed", err))
		} else {
			fmt.Println(i18n.T("config_saved"))
		}
		return
	}
}
```

**`internal/cli/list.go`** —— 提取 `printAccounts` 供菜单复用（`RunE` 主体抽成函数，CLI 输出不变）：

```go
func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all accounts",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			printAccounts(os.Stdout, bootstrap().Cfg)
			return nil
		},
	}
}

// printAccounts 渲染账号表格，CLI list 与交互菜单共用。
func printAccounts(w io.Writer, cfg *models.AppConfig) {
	if len(cfg.Accounts) == 0 {
		fmt.Fprintln(w, i18n.T("list_empty"))
		return
	}
	sortAccounts(cfg)
	now := time.Now()
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tACTIVE\tNAME\tUSER\tPLAN\tUSAGE\tRESET\tSTATUS")
	for i := range cfg.Accounts {
		a := &cfg.Accounts[i]
		mark := ""
		if a.ID == cfg.ActiveID() {
			mark = "*"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%.0f%%\t%s\t%s\n",
			i+1, mark, a.AuthData.User.Name, a.AuthData.User.Username,
			a.PlanName, a.UsagePercent, resetCol(a, now), stateOf(a, now))
	}
	_ = tw.Flush()
}

// stateOf 对应原版 AccountVisualState（Invalid/Expired/Expiring/Normal）
func stateOf(a *models.ManagedAccount, now time.Time) string {
	switch {
	case !a.IsValid:
		return "INVALID"
	case timeutil.IsExpired(a.ResetTime, now):
		return "EXPIRED"
	case timeutil.IsExpiringSoon(a.ResetTime, now):
		return "EXPIRING"
	default:
		return "OK"
	}
}

func resetCol(a *models.ManagedAccount, now time.Time) string {
	t, ok := timeutil.ParseResetDatetime(a.ResetTime, now)
	if !ok {
		return a.ResetTime
	}
	d := t.Sub(now)
	if d <= 0 {
		return "overdue"
	}
	if d >= time.Hour {
		return fmt.Sprintf("in %dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("in %dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// sortAccounts：激活账号永远第一；stale（无法解析/已过期）排最后；
// ResetTime 主键 + 余量次键（或反之），与原版 sort_accounts 一致。
func sortAccounts(cfg *models.AppConfig) {
	now := time.Now()
	activeID := cfg.ActiveID()
	isActive := func(a *models.ManagedAccount) bool { return a.ID == activeID }
	stale := func(a *models.ManagedAccount) bool {
		t, ok := timeutil.ParseResetDatetime(a.ResetTime, now)
		return !ok || t.Before(now)
	}
	remaining := func(a *models.ManagedAccount) float64 { return 100 - float64(a.UsagePercent) }
	sort.SliceStable(cfg.Accounts, func(i, j int) bool {
		a, b := &cfg.Accounts[i], &cfg.Accounts[j]
		if cfg.SortMode == models.SortNone {
			return a.AuthData.User.Name < b.AuthData.User.Name
		}
		if ai, bi := isActive(a), isActive(b); ai != bi {
			return ai
		}
		if sa, sb := stale(a), stale(b); sa != sb {
			return !sa
		}
		ta, _ := timeutil.ParseResetDatetime(a.ResetTime, now)
		tb, _ := timeutil.ParseResetDatetime(b.ResetTime, now)
		ra, rb := remaining(a), remaining(b)
		if cfg.SortMode == models.SortResetTime {
			if !ta.Equal(tb) {
				return ta.Before(tb)
			}
			return rb < ra
		}
		if ra != rb {
			return ra > rb
		}
		return ta.Before(tb)
	})
}
```

**`internal/cli/cli.go`** —— 无参数时默认进入菜单（非 TTY 回退帮助）：

```go
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
		// 无参数：TTY 下直接进入交互菜单（服务器上敲一个二进制名即可）；
		// 非 TTY（脚本/CI/管道）保持旧行为显示帮助，向后兼容。
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

// bootstrap：解析数据目录 → 加载配置（失败已备份回退并告警）→ 初始化语言。
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
```

**i18n 新增 key（菜单相关，两个语言文件同步，`TestLanguageKeysAligned` 会校验）**：

```go
// zh 追加（en 同步追加英文，键一一对应）
"menu_title":                "AtomCode 账号管理",
"menu_item_list":            "查看账号（共 %d 个）",
"menu_item_switch":          "切换账号",
"menu_item_update":          "更新账号信息（执行 /login）",
"menu_item_sync":            "同步当前 auth.toml",
"menu_item_auto_switch":     "立即检查自动切换（当前: %s, 阈值 %.0f%%）",
"menu_item_delete":          "删除账号",
"menu_item_export":          "导出账号",
"menu_item_import_file":     "从文件导入账号",
"menu_item_settings":        "设置（激活: %s）",
"menu_item_quit":            "退出",
"menu_choose_hint":          "↑/↓ 选择, Enter 确认, q 取消",
"menu_press_enter":          "按回车返回…",
"menu_requires_tty":         "交互菜单需要终端（TTY）；请改用子命令，或在终端中运行 menu",
"menu_confirm_title":        "请确认",
"menu_yes":                  "确认", "menu_no": "取消",
"menu_on":                   "开", "menu_off": "关",
"menu_back":                 "返回主菜单",
"menu_switch_title":         "选择要切换到的账号",
"menu_delete_title":         "选择要删除的账号",
"menu_export_prompt":        "导出文件名",
"menu_import_prompt":        "输入 YAML 文件路径",
"menu_settings_title":       "设置",
"menu_settings_language":    "语言（当前: %s）",
"menu_settings_sort":        "排序方式（当前: %s）",
"menu_settings_auto_switch": "自动切换（当前: %s）",
"menu_settings_threshold":   "自动切换阈值（当前: %.0f%%）",
"menu_threshold_prompt":     "输入新阈值（50-100，回车取消）",
"auto_switch_none":          "当前无需自动切换",
```

### 6.4 边界情况与测试

| 场景 | 处理 |
|---|---|
| 非 TTY（cron/管道/CI） | `newTTY` 跳过 raw 模式；`menu` 返回 `menu_requires_tty`；根命令无参数时显示帮助而非进菜单 |
| 获取 `/login` 期间按 Ctrl+C | raw 模式下 Ctrl+C 是字节 `0x03` 不产生信号，会缓存到 stdin，fetch 结束后下一次 `readKey` 返回 `keyQuit` 退出菜单（可接受；fetch 本身有 10s 超时） |
| 行输入（阈值/文件名） | `canonical()` 临时恢复规范模式，输入完重新进 raw（**必须重新调 `enableRawModeExtras`**，否则 Windows 下方向键失效） |
| 并发进程改配置 | 每轮循环重新 `bootstrap()` |
| 可单测部分 | `menuItems`（含激活名/账号数）、`onOff`；交互循环不写自动化测试，手工验收矩阵：中英文切换、四类账号状态下选择器显示、取消路径（q）不落盘 |

测试补充：

```go
func TestNewRootRegistersCommands(t *testing.T) {
	// 在已有列表中加入 "menu"
}

func TestMenuItemsShowsCountAndActive(t *testing.T) {
	svc := ops.New(models.DefaultConfig(), store.New(t.TempDir()))
	svc.Cfg.Accounts = []models.ManagedAccount{{ID: "a", AuthData: models.AuthToml{User: models.User{Name: "Alice"}}, IsValid: true}}
	svc.Cfg.SetActiveID("a")
	items := menuItems(svc)
	if len(items) != 10 {
		t.Fatalf("menu items = %d, want 10", len(items))
	}
	if !strings.Contains(items[0], "1") || !strings.Contains(items[8], "Alice") {
		t.Fatalf("unexpected menu items: %v", items)
	}
}
```

---

## 七、核心代码

> 以下代码按包组织，均对齐 `go-atomcode-switch-cli` 仓库当前实现（非初版文档的早期草稿）。其中 `cli.go` / `list.go` / `menu.go` / `tui*.go` 已并入第六节（交互菜单），本节 7.10 只补其余子命令。

### 7.1 go.mod 与入口

```text
module go-atomcode-switch-cli

go 1.27.3

require (
	github.com/BurntSushi/toml v1.4.0
	github.com/spf13/cobra v1.8.1
	golang.org/x/sys   v0.28.0
	golang.org/x/term  v0.27.0
	gopkg.in/yaml.v3   v3.0.1
)
```

```go
// cmd/atomcode-switch-cli/main.go
package main

import (
	"fmt"
	"os"

	"go-atomcode-switch-cli/internal/cli"
)

var version = "dev" // -ldflags "-X main.version=vX.Y.Z" 注入

func main() {
	cli.SetVersion(version)
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

### 7.2 models —— 结构体 + 旧配置兼容 + 激活账号访问器

```go
// internal/models/models.go
package models

import (
	"math"
	"strings"

	"gopkg.in/yaml.v3"
)

// AuthToml 对应 ~/.atomcode/auth.toml（toml 与 yaml 双用）。
type AuthToml struct {
	AccessToken  string `toml:"access_token" yaml:"access_token"`
	RefreshToken string `toml:"refresh_token" yaml:"refresh_token"`
	TokenType    string `toml:"token_type" yaml:"token_type"`
	ExpiresIn    uint64 `toml:"expires_in" yaml:"expires_in"`
	CreatedAt    uint64 `toml:"created_at" yaml:"created_at"`
	User         User   `toml:"user" yaml:"user"`
}

// User 是 auth.toml 中的用户信息。
type User struct {
	ID        string `toml:"id" yaml:"id"`
	Username  string `toml:"username" yaml:"username"`
	Name      string `toml:"name" yaml:"name"`
	Email     string `toml:"email" yaml:"email"`
	AvatarURL string `toml:"avatar_url,omitempty" yaml:"avatar_url,omitempty"`
}

// SortMode 保持与 Rust/serde 相同的序列化值。
type SortMode string

const (
	SortNone            SortMode = "None"
	SortResetTime       SortMode = "ResetTime"
	SortWindowRemaining SortMode = "WindowRemaining"
)

// UnmarshalYAML 兼容大小写和下划线写法，非法值回退到 None。
func (m *SortMode) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "resettime", "reset_time":
		*m = SortResetTime
	case "windowremaining", "window_remaining":
		*m = SortWindowRemaining
	default:
		*m = SortNone
	}
	return nil
}

// InfoStatus 表示账号信息是否经过 /login 解析刷新。
type InfoStatus string

const (
	StatusUnknown InfoStatus = "Unknown" // 刚导入，未跑过 /login，不参与自动切换
	StatusFresh   InfoStatus = "Fresh"   // 已通过 /login 解析刷新
)

// UnmarshalYAML 兼容大小写，非法值回退到 Unknown。
func (m *InfoStatus) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(s), string(StatusFresh)) {
		*m = StatusFresh
	} else {
		*m = StatusUnknown
	}
	return nil
}

// AppConfig 对应 atomcode-accounts.yaml。
type AppConfig struct {
	CustomAtomcodeDir         *string          `yaml:"custom_atomcode_dir,omitempty"`
	ActiveAccountID           *string          `yaml:"active_account_id,omitempty"`
	Accounts                  []ManagedAccount `yaml:"accounts"`
	AutoSwitchRules           AutoSwitchRules  `yaml:"auto_switch_rules"`
	Language                  string           `yaml:"language"`
	AutoUpdateOnSwitchExpired bool             `yaml:"auto_update_on_switch_expired"`
	SortMode                  SortMode         `yaml:"sort_mode"`
}

// ManagedAccount 是受管理的账号记录。
type ManagedAccount struct {
	ID            string     `yaml:"id"`
	AuthData      AuthToml   `yaml:"auth_data"`
	PlanName      string     `yaml:"plan_name"`
	UsagePercent  float32    `yaml:"usage_percent"`
	ResetTime     string     `yaml:"reset_time"`
	IsValid       bool       `yaml:"is_valid"`
	RemainingDays uint32     `yaml:"remaining_days"`
	LastUpdated   string     `yaml:"last_updated"`
	InfoStatus    InfoStatus `yaml:"info_status"`
}

type managedAccountAlias ManagedAccount

// UnmarshalYAML 补齐旧账号缺失的 info_status 字段。
func (m *ManagedAccount) UnmarshalYAML(node *yaml.Node) error {
	var aux managedAccountAlias
	if err := node.Decode(&aux); err != nil {
		return err
	}
	*m = ManagedAccount(aux)
	if m.InfoStatus == "" {
		m.InfoStatus = StatusUnknown
	}
	return nil
}

// AutoSwitchRules 是自动切换阈值配置。
type AutoSwitchRules struct {
	Enabled         bool    `yaml:"enabled"`
	MaxUsagePercent float32 `yaml:"max_usage_percent"`
}

// DefaultConfig 返回与原 Rust AppConfig::default 一致的配置。
func DefaultConfig() *AppConfig {
	return &AppConfig{
		Accounts:                  []ManagedAccount{},
		AutoSwitchRules:           AutoSwitchRules{Enabled: false, MaxUsagePercent: 95.0},
		Language:                  "zh_cn",
		AutoUpdateOnSwitchExpired: false,
		SortMode:                  SortNone,
	}
}

// Normalize 补齐旧配置缺失字段；合法旧值保持不变。
func (c *AppConfig) Normalize() {
	if strings.TrimSpace(c.Language) == "" {
		c.Language = "zh_cn"
	}
	if !validPercent(c.AutoSwitchRules.MaxUsagePercent) {
		c.AutoSwitchRules.MaxUsagePercent = 95.0
	}
	if c.Accounts == nil {
		c.Accounts = []ManagedAccount{}
	}
	if c.SortMode == "" {
		c.SortMode = SortNone
	}
	for i := range c.Accounts {
		if c.Accounts[i].InfoStatus == "" {
			c.Accounts[i].InfoStatus = StatusUnknown
		}
	}
}

func validPercent(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v > 0 && v <= 100
}

// AtomcodeDir 返回自定义的 auth.toml 目录；空串表示使用默认 ~/.atomcode。
func (c *AppConfig) AtomcodeDir() string {
	if c.CustomAtomcodeDir == nil {
		return ""
	}
	return *c.CustomAtomcodeDir
}

// SetAtomcodeDir 设置或清除自定义 auth.toml 目录。
func (c *AppConfig) SetAtomcodeDir(dir string) {
	if dir == "" {
		c.CustomAtomcodeDir = nil
		return
	}
	c.CustomAtomcodeDir = &dir
}

// ActiveID 返回当前激活账号 ID；没有激活账号时返回空串。
func (c *AppConfig) ActiveID() string {
	if c.ActiveAccountID == nil {
		return ""
	}
	return *c.ActiveAccountID
}

// SetActiveID 设置当前激活账号 ID。
func (c *AppConfig) SetActiveID(id string) {
	c.ActiveAccountID = &id
}
```

### 7.3 timeutil —— 统一时间解析

```go
// internal/timeutil/timeutil.go
package timeutil

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LayoutFull 是重置时间的完整格式。
const LayoutFull = "2006-01-02 15:04"

// ParseHHMM 严格解析 HH:MM，仅接受两位 ASCII 数字。
func ParseHHMM(raw string) (int, int, bool) {
	t := strings.TrimSpace(raw)
	if len(t) < 5 || t[2] != ':' {
		return 0, 0, false
	}
	for _, i := range [4]int{0, 1, 3, 4} {
		if t[i] < '0' || t[i] > '9' {
			return 0, 0, false
		}
	}
	h, err1 := strconv.Atoi(t[:2])
	m, err2 := strconv.Atoi(t[3:5])
	if err1 != nil || err2 != nil || h >= 24 || m >= 60 {
		return 0, 0, false
	}
	return h, m, true
}

// ParseResetDatetime 是重置时间的统一解析入口。
// 新格式直接使用；旧 HH:MM 结合 now 的日期，若已到或已过则顺延到明天。
func ParseResetDatetime(s string, now time.Time) (time.Time, bool) {
	t := strings.TrimSpace(s)
	if dt, err := time.ParseInLocation(LayoutFull, t, now.Location()); err == nil {
		return dt, true
	}
	h, m, ok := ParseHHMM(t)
	if !ok {
		return time.Time{}, false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if today.After(now) {
		return today, true
	}
	return today.AddDate(0, 0, 1), true
}

// CombineResetDatetime 把 HH:MM 组合为完整的 YYYY-MM-DD HH:MM（已过顺延明天）。
func CombineResetDatetime(hm string, now time.Time) (string, bool) {
	h, m, ok := ParseHHMM(hm)
	if !ok {
		return "", false
	}
	targetSecs := h*3600 + m*60
	nowSecs := now.Hour()*3600 + now.Minute()*60 + now.Second()
	day := now
	if targetSecs <= nowSecs {
		day = day.AddDate(0, 0, 1)
	}
	return fmt.Sprintf("%s %02d:%02d", day.Format("2006-01-02"), h, m), true
}

// IsExpired 判断重置时间是否已经过去。
func IsExpired(resetTime string, now time.Time) bool {
	t, ok := ParseResetDatetime(resetTime, now)
	return ok && t.Before(now)
}

// IsExpiringSoon 判断是否在一小时内重置。
func IsExpiringSoon(resetTime string, now time.Time) bool {
	t, ok := ParseResetDatetime(resetTime, now)
	return ok && !t.Before(now) && t.Sub(now) < time.Hour
}
```

### 7.4 parser —— `/login` 输出解析（严格校验 + 零字段不落盘）

> 实现从初版文档的正则方案演进为**标记/终止符字符串扫描**：对套餐、剩余天数、用量、重置时间分别按标记定位并严格校验，未识别出的字段不会置位，从而保证"零字段不落盘"。

```go
// internal/parser/parser.go
package parser

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"go-atomcode-switch-cli/internal/timeutil"
)

// Fields 保存从 /login 输出中识别出的字段；每个字段都必须通过严格校验。
type Fields struct {
	PlanName      string
	UsagePercent  float32
	ResetTime     string
	RemainingDays uint32

	HasPlan  bool
	HasUsage bool
	HasReset bool
	HasDays  bool
}

// Empty 返回一个字段都没有识别出。
func (f Fields) Empty() bool {
	return !f.HasPlan && !f.HasUsage && !f.HasReset && !f.HasDays
}

// Parse 解析 /login 输出，优先复刻 Rust 版的标记和终止符，同时兼容少量常见变体。
func Parse(text string, now time.Time) Fields {
	var f Fields

	if v, ok := firstPlan(text, "套餐：", "套餐:", "Plan:", "Plan："); ok {
		f.PlanName, f.HasPlan = v, true
	}

	if v, ok := numberAfter(text, "剩余 ", false); ok {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			f.RemainingDays, f.HasDays = uint32(n), true
		}
	}
	if !f.HasDays {
		if n, ok := remainingDaysEN(text); ok {
			f.RemainingDays, f.HasDays = n, true
		}
	}

	for _, marker := range []string{"用量约", "Usage:", "usage:"} {
		v, ok := numberAfter(text, marker, true)
		if !ok {
			continue
		}
		p, err := strconv.ParseFloat(v, 32)
		if err == nil && p >= 0 && p <= 100 {
			f.UsagePercent, f.HasUsage = float32(p), true
			break
		}
	}

	for _, keyword := range []string{"重置于"} {
		for _, line := range strings.Split(text, "\n") {
			if v, ok := resetTimeInLine(line, keyword, now); ok {
				f.ResetTime, f.HasReset = v, true
				break
			}
		}
		if f.HasReset {
			break
		}
	}
	if !f.HasReset {
		for _, keyword := range []string{"resets ", "resets\t"} {
			for _, line := range strings.Split(text, "\n") {
				if v, ok := resetTimeInLine(line, keyword, now); ok {
					f.ResetTime, f.HasReset = v, true
					break
				}
			}
			if f.HasReset {
				break
			}
		}
	}

	return f
}

func firstPlan(text string, markers ...string) (string, bool) {
	for _, marker := range markers {
		if v, ok := planAfter(text, marker); ok {
			return v, true
		}
	}
	return "", false
}

func planAfter(text, marker string) (string, bool) {
	pos := strings.Index(text, marker)
	if pos < 0 {
		return "", false
	}
	after := text[pos+len(marker):]
	end := len(after)
	for _, terminator := range []string{"·", "（", "(", "到期时间", "expires", "\r", "\n"} {
		if i := strings.Index(after, terminator); i >= 0 && i < end {
			end = i
		}
	}
	value := strings.TrimSpace(after[:end])
	return value, value != ""
}

func numberAfter(text, marker string, allowDot bool) (string, bool) {
	pos := strings.Index(text, marker)
	if pos < 0 {
		return "", false
	}
	rest := strings.TrimLeftFunc(text[pos+len(marker):], unicode.IsSpace)
	var b strings.Builder
	for _, r := range rest {
		if (r >= '0' && r <= '9') || (allowDot && r == '.') {
			b.WriteRune(r)
			continue
		}
		break
	}
	v := b.String()
	return v, v != ""
}

func remainingDaysEN(text string) (uint32, bool) {
	for _, line := range strings.Split(text, "\n") {
		pos := strings.Index(line, "d /")
		if pos < 0 {
			continue
		}
		before := line[:pos]
		i := len(before)
		for i > 0 && before[i-1] >= '0' && before[i-1] <= '9' {
			i--
		}
		if i == len(before) {
			continue
		}
		n, err := strconv.ParseUint(before[i:], 10, 32)
		if err == nil {
			return uint32(n), true
		}
	}
	return 0, false
}

func resetTimeInLine(line, keyword string, now time.Time) (string, bool) {
	parts := strings.SplitN(line, keyword, 2)
	if len(parts) != 2 {
		return "", false
	}
	return timeutil.CombineResetDatetime(strings.TrimSpace(parts[1]), now)
}
```

### 7.5 store —— 原子写 / 加载保存 / 旧数据迁移 / 导出导入 / 数据目录解析

```go
// internal/store/store.go
package store

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"go-atomcode-switch-cli/internal/models"

	"gopkg.in/yaml.v3"
)

const (
	accountsFileName = "atomcode-accounts.yaml"
	localConfigName  = "config.yaml"
)

var exeDirForTest string // 测试注入点，改动 store 时保持该 hook

// LocalConfig 是 exe 旁 config.yaml 的内容。
type LocalConfig struct {
	CustomDataDir string `yaml:"custom_data_dir,omitempty"`
}

// HomeDir 返回用户主目录，Windows 下优先 USERPROFILE。
func HomeDir() string {
	var primary, fallback string
	if runtime.GOOS == "windows" {
		primary, fallback = os.Getenv("USERPROFILE"), os.Getenv("HOME")
	} else {
		primary, fallback = os.Getenv("HOME"), os.Getenv("USERPROFILE")
	}
	for _, v := range []string{primary, fallback} {
		if v != "" {
			return v
		}
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return "."
}

func exeDir() string {
	if exeDirForTest != "" {
		return exeDirForTest
	}
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); dir != "" {
			return dir
		}
	}
	return "."
}

// LocalConfigPath 返回 exe 旁 config.yaml 的路径。
func LocalConfigPath() string {
	return filepath.Join(exeDir(), localConfigName)
}

// BackupFile 为已存在文件写入 .bak 副本。
func BackupFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak", data, 0o600)
}

// AtomicWriteText 使用 .tmp + fsync + rename 原子写入文本文件。
// keepBackup=true 时先保留 .bak；auth.toml 写入应传 false。
func AtomicWriteText(path, content string, keepBackup bool) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if keepBackup {
		_ = BackupFile(path)
	}
	if err := os.Rename(tmp, path); err != nil {
		if werr := os.WriteFile(path, []byte(content), 0o600); werr != nil {
			_ = os.Remove(tmp)
			return werr
		}
		_ = os.Remove(tmp)
	}
	return nil
}

// Store 绑定一个数据目录。
type Store struct {
	dataDir string
}

// New 创建 Store。
func New(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// DataDir 返回绑定的数据目录。
func (s *Store) DataDir() string {
	return s.dataDir
}

// AccountsPath 返回账号配置文件路径。
func (s *Store) AccountsPath() string {
	return filepath.Join(s.dataDir, accountsFileName)
}

// Load 读取配置；解析失败会备份原文件并返回默认配置与错误（不静默丢数据）。
func (s *Store) Load() (*models.AppConfig, error) {
	s.migrateLegacyIfNeeded()
	path := s.AccountsPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return models.DefaultConfig(), nil
	}
	if err != nil {
		return models.DefaultConfig(), fmt.Errorf("read %s: %w", path, err)
	}
	var cfg models.AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		_ = BackupFile(path)
		return models.DefaultConfig(), fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.Normalize()
	return &cfg, nil
}

// Save 保存账号配置。
func (s *Store) Save(cfg *models.AppConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return AtomicWriteText(s.AccountsPath(), string(data), true)
}

// Export 导出账号配置。
func (s *Store) Export(path string, cfg *models.AppConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return AtomicWriteText(path, string(data), true)
}

// Import 读取并校验导入文件，不修改当前配置。
func (s *Store) Import(path string) (*models.AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg models.AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.Normalize()
	return &cfg, nil
}

// migrateLegacyIfNeeded：~/.atomcode-switch/config.yaml → atomcode-accounts.yaml
func (s *Store) migrateLegacyIfNeeded() {
	oldPath := filepath.Join(HomeDir(), ".atomcode-switch", "config.yaml")
	if _, err := os.Stat(oldPath); err != nil {
		return
	}
	newPath := s.AccountsPath()
	if _, err := os.Stat(newPath); err == nil {
		return
	}
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return
	}
	if len(bytes.TrimSpace(data)) == 0 {
		_ = os.Remove(oldPath)
		return
	}
	var probe models.AppConfig
	if yaml.Unmarshal(data, &probe) == nil {
		if AtomicWriteText(newPath, string(data), false) == nil {
			_ = os.Remove(oldPath)
		}
	} else {
		_ = BackupFile(oldPath)
	}
}

// ResolveDataDir 优先级：flag > 环境变量 > exe 旁 config.yaml（兼容原版）> ~/.atomcode-switch
func ResolveDataDir(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv("ATOMCODE_SWITCH_DATA_DIR"); v != "" {
		return v
	}
	if dir := loadLocalConfig().CustomDataDir; dir != "" {
		return dir
	}
	return filepath.Join(HomeDir(), ".atomcode-switch")
}

// LoadLocalConfig 读取 exe 旁 config.yaml；解析失败会备份并回退默认值。
func LoadLocalConfig() LocalConfig {
	return loadLocalConfig()
}

func loadLocalConfig() LocalConfig {
	path := LocalConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return LocalConfig{}
	}
	var lc LocalConfig
	if err := yaml.Unmarshal(data, &lc); err != nil {
		_ = BackupFile(path)
		return LocalConfig{}
	}
	return lc
}

// SaveLocalConfig 保存或清除 exe 旁 config.yaml 的自定义数据目录。
func SaveLocalConfig(dir string) error {
	path := LocalConfigPath()
	if dir == "" {
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		return os.Remove(path)
	}
	lc := loadLocalConfig()
	lc.CustomDataDir = dir
	data, err := yaml.Marshal(&lc)
	if err != nil {
		return err
	}
	return AtomicWriteText(path, string(data), true)
}
```

### 7.6 atomio —— auth.toml 读写 + 用户名快照

```go
// internal/atomio/atomio.go
package atomio

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"go-atomcode-switch-cli/internal/models"
	"go-atomcode-switch-cli/internal/store"

	"github.com/BurntSushi/toml"
)

// ErrAuthNotFound 统一表示当前 auth.toml 不存在、不可读或格式损坏。
var ErrAuthNotFound = errors.New("auth.toml not found")

// Dir 返回 auth.toml 所在目录。
func Dir(custom string) string {
	if custom != "" {
		return custom
	}
	return filepath.Join(store.HomeDir(), ".atomcode")
}

// AuthPath 返回 auth.toml 的完整路径。
func AuthPath(custom string) string {
	return filepath.Join(Dir(custom), "auth.toml")
}

// Read 读取当前 auth.toml；任何失败都归一化为 ErrAuthNotFound。
func Read(custom string) (*models.AuthToml, error) {
	data, err := os.ReadFile(AuthPath(custom))
	if err != nil {
		return nil, ErrAuthNotFound
	}
	var auth models.AuthToml
	if _, err := toml.Decode(string(data), &auth); err != nil {
		slog.Debug("auth.toml decode failed", "err", err)
		return nil, ErrAuthNotFound
	}
	return &auth, nil
}

// Write 原子写 auth.toml（无 .bak，切换立即生效），并 best-effort 写入按用户名命名的快照。
func Write(custom string, auth *models.AuthToml) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(auth); err != nil { // omitempty：空 avatar_url 不写出
		return err
	}
	content := buf.String()
	if err := store.AtomicWriteText(AuthPath(custom), content, false); err != nil {
		return err
	}
	if name := snapshotName(auth); name != "" {
		snapshot := filepath.Join(Dir(custom), "auth.toml."+name)
		if err := store.AtomicWriteText(snapshot, content, false); err != nil {
			slog.Warn("auth snapshot failed", "path", snapshot, "err", err)
		}
	}
	return nil
}

// IsNotExist 判断错误是否表示当前 auth 不可用。
func IsNotExist(err error) bool {
	return errors.Is(err, ErrAuthNotFound)
}

// snapshotName 过滤非法字符，得到安全的快照文件名。
func snapshotName(auth *models.AuthToml) string {
	raw := auth.User.Username
	if raw == "" {
		raw = auth.User.Name
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
```

### 7.7 fetch —— 执行 `atomcode login`（替代后台线程方案）

```go
// internal/fetch/fetch.go
package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	// ErrTimeout 表示获取登录信息超时。
	ErrTimeout = errors.New("fetch timeout")
	// ErrNoOutput 表示子进程没有产生任何输出。
	ErrNoOutput = errors.New("no output")
)

const pollInterval = 100 * time.Millisecond

// 关键词早退：字段齐备信号，覆盖中英文（与 Rust 版关键词一致）。
var earlyKeywords = []string{
	"套餐", "Plan:", "剩余", "Usage", "usage", "到期时间", "resets", "重置于",
}

// syncBuf 线程安全缓冲：os/exec 会为 io.Writer 自动开 pipe + copy goroutine，
// Wait 返回时保证 copy 完成、缓冲完整 —— 无需 Rust 版的手动 flush 窗口。
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuf) ContainsAny(keywords ...string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.buf.String()
	for _, keyword := range keywords {
		if strings.Contains(s, keyword) {
			return true
		}
	}
	return false
}

// FetchLoginOutput 执行 atomcode login，捕获 stdout/stderr 并支持关键词早退与超时终止。
func FetchLoginOutput(parent context.Context, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "atomcode", "login")
	cmd.Stdin = nil // login 进交互模式时立即 EOF
	var stdout, stderr syncBuf
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("exec atomcode login: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	exited := false
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-waitCh:
			exited = true
			break loop
		case <-tick.C:
			if stdout.ContainsAny(earlyKeywords...) || stderr.ContainsAny(earlyKeywords...) {
				break loop
			}
		}
	}

	if !exited {
		_ = cmd.Process.Kill()
		<-waitCh // 等待 Wait：内部 copy goroutine 已排空管道
	}

	combined := strings.TrimSpace(stdout.String())
	if e := strings.TrimSpace(stderr.String()); e != "" {
		if combined == "" {
			combined = e
		} else {
			combined += "\n" + e
		}
	}
	if combined == "" {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", ErrTimeout
		}
		return "", ErrNoOutput
	}
	return combined, nil
}
```

### 7.8 ops —— 业务逻辑（唯一业务层）

```go
// internal/ops/ops.go
package ops

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go-atomcode-switch-cli/internal/atomio"
	"go-atomcode-switch-cli/internal/i18n"
	"go-atomcode-switch-cli/internal/models"
	"go-atomcode-switch-cli/internal/parser"
	"go-atomcode-switch-cli/internal/store"
)

var (
	ErrNotFound       = errors.New("account not found")
	ErrNoFields       = errors.New("no recognizable account fields")
	ErrUserUnmatched  = errors.New("login user does not match any account")
	ErrNoActive       = errors.New("no active account")
	ErrActiveNotFound = errors.New("active account not found in account list")
	ErrAuthNotFound   = errors.New("auth.toml not found")
)

const hysteresis = 5.0 // 滞回：目标账号须比当前低至少 5%，防止来回抖动

// Service 封装账号业务事务。
type Service struct {
	Cfg     *models.AppConfig
	Store   *store.Store
	DataDir string
}

// New 创建业务服务。
func New(cfg *models.AppConfig, st *store.Store) *Service {
	return &Service{Cfg: cfg, Store: st, DataDir: st.DataDir()}
}

// Save 持久化当前配置。
func (s *Service) Save() error {
	return s.Store.Save(s.Cfg)
}

// FindByID 按 ID 查找账号。
func (s *Service) FindByID(id string) *models.ManagedAccount {
	for i := range s.Cfg.Accounts {
		if s.Cfg.Accounts[i].ID == id {
			return &s.Cfg.Accounts[i]
		}
	}
	return nil
}

// Active 返回当前激活账号。
func (s *Service) Active() *models.ManagedAccount {
	return s.FindByID(s.Cfg.ActiveID())
}

// Resolve 支持 1-based 序号、账号 ID、名称和用户名。
func (s *Service) Resolve(pattern string) (*models.ManagedAccount, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, ErrNotFound
	}
	if n, err := strconv.Atoi(pattern); err == nil {
		if n >= 1 && n <= len(s.Cfg.Accounts) {
			return &s.Cfg.Accounts[n-1], nil
		}
		return nil, fmt.Errorf("%w: index %d (1..%d)", ErrNotFound, n, len(s.Cfg.Accounts))
	}
	for i := range s.Cfg.Accounts {
		acc := &s.Cfg.Accounts[i]
		if acc.ID == pattern || acc.AuthData.User.Name == pattern || acc.AuthData.User.Username == pattern {
			return acc, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrNotFound, pattern)
}

// Switch 写入目标账号 auth.toml 并更新激活账号。
func (s *Service) Switch(id string) error {
	acc := s.FindByID(id)
	if acc == nil {
		return ErrNotFound
	}
	if err := atomio.Write(s.Cfg.AtomcodeDir(), &acc.AuthData); err != nil {
		return fmt.Errorf("write auth.toml: %w", err)
	}
	s.Cfg.SetActiveID(id)
	return s.Save()
}

// SyncCurrentAuthExisting 仅同步已存在账号的凭据（不新增、无激活账号概念），
// 供 daemon 启动时使用：当前 auth.toml 的账号不在列表中则不动。
func (s *Service) SyncCurrentAuthExisting() error {
	auth, err := atomio.Read(s.Cfg.AtomcodeDir())
	if err != nil {
		return nil
	}
	id := auth.User.ID
	for i := range s.Cfg.Accounts {
		if s.Cfg.Accounts[i].ID != id {
			continue
		}
		s.Cfg.Accounts[i].AuthData = *auth
		s.Cfg.SetActiveID(id)
		return s.Save()
	}
	return nil
}

// ImportCurrentAuth 从当前 auth.toml 导入/同步（新账号 info_status=Unknown，不参与自动切换）。
func (s *Service) ImportCurrentAuth() (isNew bool, name string, err error) {
	auth, err := atomio.Read(s.Cfg.AtomcodeDir())
	if err != nil {
		return false, "", ErrAuthNotFound
	}
	id := auth.User.ID
	for i := range s.Cfg.Accounts {
		if s.Cfg.Accounts[i].ID != id {
			continue
		}
		s.Cfg.Accounts[i].AuthData = *auth
		s.Cfg.Accounts[i].LastUpdated = NowStr()
		s.Cfg.SetActiveID(id)
		return false, auth.User.Name, s.Save()
	}
	s.Cfg.Accounts = append(s.Cfg.Accounts, models.ManagedAccount{
		ID:           id,
		AuthData:     *auth,
		PlanName:     i18n.T("plan_unknown"),
		UsagePercent: 0,
		ResetTime:    DefaultResetTime(),
		IsValid:      true,
		LastUpdated:  NowStr(),
		InfoStatus:   models.StatusUnknown,
	})
	s.Cfg.SetActiveID(id)
	return true, auth.User.Name, s.Save()
}

// Delete 删除账号；删除激活账号后自动切换到第一个剩余账号。
func (s *Service) Delete(id string) error {
	idx := -1
	for i := range s.Cfg.Accounts {
		if s.Cfg.Accounts[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	s.Cfg.Accounts = append(s.Cfg.Accounts[:idx], s.Cfg.Accounts[idx+1:]...)
	if s.Cfg.ActiveID() == id {
	 s.Cfg.ActiveAccountID = nil
	 if len(s.Cfg.Accounts) > 0 {
	  return s.Switch(s.Cfg.Accounts[0].ID)
	 }
	}
	return s.Save()
}

// CheckAutoSwitch 按阈值、5% 滞回和 Fresh 限定检查并执行自动切换。
func (s *Service) CheckAutoSwitch() (string, bool) {
	r := &s.Cfg.AutoSwitchRules
	if !r.Enabled {
	 return "", false
	}
	active := s.Active()
	if active == nil || active.UsagePercent < r.MaxUsagePercent {
	 return "", false
	}

	var best *models.ManagedAccount
	for i := range s.Cfg.Accounts {
	 acc := &s.Cfg.Accounts[i]
	 if acc.ID == active.ID || !acc.IsValid || acc.InfoStatus != models.StatusFresh {
	  continue
	 }
	 if acc.UsagePercent >= r.MaxUsagePercent {
	  continue
	 }
	 if active.UsagePercent-acc.UsagePercent < hysteresis {
	  continue
	 }
	 if best == nil || acc.UsagePercent < best.UsagePercent {
	  best = acc
	 }
	}
	if best == nil {
	 return "", false
	}

	id, name := best.ID, best.AuthData.User.Name
	if err := s.Switch(id); err != nil {
	 slog.Warn("auto switch failed", "err", err)
	 return "", false
	}
	return name, true
}

// ApplyLoginOutput：解析 /login 输出并更新账号。
//  - 按输出中的用户名匹配账号；识别到用户名但无匹配 → 拒绝落盘（防串号）；
//  - 零字段不落盘；所有字段校验通过后一次性应用（事务性）；
//  - 目标 ≠ 激活账号时，先把匹配账号凭据写盘再切 active，
//    防止随后的 ImportCurrentAuth 撤销切换（对应 Rust 版修复的回归）。
func (s *Service) ApplyLoginOutput(text string) (string, error) {
	active := s.Active()
	if active == nil {
	 if s.Cfg.ActiveID() == "" {
	  return "", ErrNoActive
	 }
	 return "", ErrActiveNotFound
	}

	target := active
	if loginName := ExtractLoginName(text); loginName != "" && !matchesLoginUser(&active.AuthData.User, loginName) {
	 found := s.findByLoginName(loginName)
	 if found == nil {
	  return "", fmt.Errorf("%w: %s", ErrUserUnmatched, loginName)
	 }
	 target = found
	}

	fields := parser.Parse(text, time.Now())
	if fields.Empty() {
	 return "", ErrNoFields
	}
	if fields.HasPlan {
	 target.PlanName = fields.PlanName
	}
	if fields.HasUsage {
	 target.UsagePercent = fields.UsagePercent
	}
	if fields.HasReset {
	 target.ResetTime = fields.ResetTime
	}
	if fields.HasDays {
	 target.RemainingDays = fields.RemainingDays
	}
	target.InfoStatus = models.StatusFresh
	target.IsValid = true
	target.LastUpdated = NowStr()

	if target != active {
	 if err := atomio.Write(s.Cfg.AtomcodeDir(), &target.AuthData); err != nil {
	  return "", fmt.Errorf("write auth.toml: %w", err)
	 }
	 s.Cfg.SetActiveID(target.ID)
	}
	if err := s.Save(); err != nil {
	 return "", err
	}
	return target.AuthData.User.Name, nil
}

func (s *Service) findByLoginName(name string) *models.ManagedAccount {
	for i := range s.Cfg.Accounts {
	 if matchesLoginUser(&s.Cfg.Accounts[i].AuthData.User, name) {
	  return &s.Cfg.Accounts[i]
	 }
	}
	return nil
}

func matchesLoginUser(user *models.User, name string) bool {
	return user.Name == name || user.Username == name
}

// ExtractLoginName 兼容中英文登录用户标记。
func ExtractLoginName(text string) string {
	markers := []string{"logged in as ", "已登录为", "登录为", "当前用户"}
	for _, marker := range markers {
	 pos := strings.Index(text, marker)
	 if pos < 0 {
	  continue
	 }
	 rest := text[pos+len(marker):]
	 rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
	 rest = strings.TrimLeft(rest, ":：")
	 rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
	 if i := strings.IndexAny(rest, "\r\n"); i >= 0 {
	  rest = rest[:i]
	 }
	 end := len(rest)
	 for i, r := range rest {
	  if unicode.IsSpace(r) || strings.ContainsRune("。（(，,:：", r) {
	   end = i
	   break
	  }
	 }
	 if name := strings.TrimSpace(rest[:end]); name != "" {
	  return name
	 }
	}
	return ""
}

// ImportFile 事务性导入账号、激活账号和自动切换规则，保留其余本地配置。
func (s *Service) ImportFile(path string) (int, error) {
	imported, err := s.Store.Import(path)
	if err != nil {
	 return 0, err
	}
	s.Cfg.Accounts = imported.Accounts
	if imported.ActiveAccountID != nil {
	 id := *imported.ActiveAccountID
	 s.Cfg.ActiveAccountID = &id
	} else {
	 s.Cfg.ActiveAccountID = nil
	}
	s.Cfg.AutoSwitchRules = imported.AutoSwitchRules
	return len(s.Cfg.Accounts), s.Save()
}

// NowStr 返回当前本地时间字符串。
func NowStr() string {
	return time.Now().Format("2006-01-02 15:04")
}

// DefaultResetTime 返回当前时间加一小时的完整时间字符串。
func DefaultResetTime() string {
	return time.Now().Add(time.Hour).Format("2006-01-02 15:04")
}
```
