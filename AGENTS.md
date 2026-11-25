# AGENTS.md

Go 编写的 AtomCode 多账号管理 CLI（模块名 `go-atomcode-switch-cli`，Go 1.27.3；直接依赖 cobra、toml、yaml.v3、`golang.org/x/term`、`golang.org/x/sys`）。

## 命令

主构建入口是 Task（`taskfile.yml`，需 Task ≥ 3.28）：

- 构建：`task build`（当前平台，输出到 `dist/atomcode-switch-cli-<os>-<arch>[.exe]`）；单平台：`task build PLATFORM=linux/arm64`
- 测试：`task test`（`go test -count=1 ./...`，绕过缓存）
- 检查：`task check`（fmt-check + vet + test，提交/CI 前跑一次）
- 竞态：`task test-race`
- 全平台交叉编译：`task build ALL=true` 或 `task build-all`（遍历 `vars.PLATFORMS`）
- 发布包：`task package`（二进制 + 归档），`task checksum`（SHA256 清单）
- 清理：`task clean`

`taskfile.yml` 是当前构建与发布入口。版本号通过 ldflags `-X main.version=<version>` 注入；所有交叉编译强制 `CGO_ENABLED=0` 与 `-trimpath`，构建后用 `go version -m` 自检目标平台、版本和 CGO 状态。

## 架构

- `cmd/atomcode-switch-cli/main.go`：唯一入口，仅注入版本并调 `cli.Execute()`。
- `internal/cli`：Cobra 命令层（menu/list/switch/import/update/delete/export/import-file/config/daemon），每个子命令一个文件；命令通过 `bootstrap()` 组装 service。
- 根命令无参数时：stdin 是 TTY 则进入交互菜单，非 TTY（脚本/CI/管道）返回帮助。显式 `menu` / `m` / `ui` 也可进入菜单。
- `internal/cli/menu.go`：交互菜单主循环与动作薄壳；所有动作复用现有 `ops.Service`、`runUpdate` 和 `Store.Export`，不新增业务规则。每次回到主菜单前重新 `bootstrap()`，以反映外部 YAML 修改或其他终端的切换。
- `internal/cli/tui.go`、`tui_windows.go`、`tui_raw_unix.go`（及 `tui_raw_linux.go` / `tui_raw_darwin.go`）、`tui_other.go`：基于 `golang.org/x/term` 的轻量 raw 终端选择器、确认框、暂停和行输入；Windows VT 输入/输出位在 `tui_windows.go` 中开启，Unix raw 后重新开启 `OPOST|ONLCR`。
- `internal/cli/list.go`：`printAccounts(io.Writer, *models.AppConfig)` 是 list 命令与菜单查看账号共用的表格渲染入口；修改列表输出时两边同时生效。
- `internal/ops`：业务逻辑层，命令层和菜单层都不含业务规则。
- `internal/store`：持久化。数据目录默认 `~/.atomcode-switch`，但 exe 旁若有 `config.yaml`（`custom_data_dir` 字段）则优先生效——排查数据位置时先看这一层。
- `internal/parser` / `internal/atomio` / `internal/fetch`：解析 AtomCode 本地凭据、与 AtomCode 交互、获取登录输出。
- `internal/i18n`：所有用户可见文案走 `i18n.T("key", ...)`，新增文案必须同时补中英文 key。

## 项目特有注意事项

- Windows 是一等公民：`store.HomeDir()` 在 Windows 下优先 `USERPROFILE` 而非 `HOME`，路径逻辑勿回退到 Unix 假设。
- 菜单使用 raw 模式：`term.MakeRaw` 后必须保存 `term.State` 并在退出时 `term.Restore`；行输入通过 `canonical()` 临时恢复规范模式，读完后重新进入 raw，并再次调用 `enableRawModeExtras()`，否则 Windows 下方向键与清屏、Unix 下换行只下移不回到行首会失效。
- 菜单要求 TTY；非 TTY 下 `menu` 子命令返回 `menu_requires_tty`，根命令无参数则返回帮助。
- 账号数据文件为 `atomcode-accounts.yaml`，含敏感凭据，不要在日志或错误信息中输出其内容；导出文件提示明文 token 风险。
- 版本号 `var version = "dev"` 由 ldflags 覆盖，不要硬编码版本字符串。
- `store.exeDirForTest` 是测试注入点，改动 store 时保持该 hook。
- Task 平台矩阵集中维护在 `vars.PLATFORMS`；新增平台优先加矩阵项，不要复制构建任务。
- 构建入口只有 `task build` / `task build-all`；`build-one`、`compile-one`、`dist-dir` 是 internal，不能从命令行直接调用。`--all` 不是自定义参数，全平台用 `ALL=true` 或 `build-all`。
- Windows 测试偶发临时测试文件被占用；必要时用 `go test -p 1 -count=1 ./...` 降低干扰。

## 维护规则

项目结构、构建/测试命令、架构边界、开发约定或本文件记录的其他事实发生变化时，必须在同一次改动中同步更新本文件：用当前事实替换过期表述，不逐次追加变更历史。本文件只保留当前规则、命令、架构边界和非显然注意事项；详细事故复盘、性能对比、实验过程和已完成迁移记录放到有链接的独立文档中，仅在任务相关时读取。篇幅目标只约束新写的指导内容，不得据此删除或摘要用户已有规则；迁移或删除已有内容前须先提出方案并取得明确同意。
