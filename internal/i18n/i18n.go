package i18n

import (
	"fmt"
	"os"
	"strings"
)

const (
	zhCN = "zh_cn"
	enUS = "en_us"
)

var lang = detect("", false)

// Init 使用配置中的语言初始化；ATOMCODE_SWITCH_LANG 可显式覆盖。
func Init(configLang string) {
	lang = detect(configLang, true)
}

// Current 返回当前规范化语言标识。
func Current() string {
	return lang
}

func detect(configLang string, allowConfig bool) string {
	if v := strings.TrimSpace(os.Getenv("ATOMCODE_SWITCH_LANG")); v != "" {
		return canonical(v)
	}
	if allowConfig && strings.TrimSpace(configLang) != "" {
		return canonical(configLang)
	}
	lc := strings.TrimSpace(os.Getenv("LC_ALL"))
	if lc == "" {
		lc = strings.TrimSpace(os.Getenv("LANG"))
	}
	lower := strings.ToLower(lc)
	if strings.Contains(lower, "zh") {
		return zhCN
	}
	if strings.Contains(lower, "en") {
		return enUS
	}
	return zhCN
}

func canonical(v string) string {
	if normalized, ok := Normalize(v); ok {
		return normalized
	}
	return zhCN
}

// Normalize 返回规范化语言标识，并报告输入是否为受支持的语言。
func Normalize(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "en", "en_us", "en-us", "english":
		return enUS, true
	case "zh", "zh_cn", "zh-cn", "chinese", "中文":
		return zhCN, true
	default:
		return "", false
	}
}

// T 返回当前语言的格式化消息；未知 key 原样返回。
func T(key string, args ...any) string {
	m := zh
	if lang == enUS {
		m = en
	}
	tpl, ok := m[key]
	if !ok {
		return key
	}
	if len(args) == 0 {
		return tpl
	}
	return fmt.Sprintf(tpl, args...)
}

var zh = map[string]string{
	"plan_unknown":              "未知套餐",
	"switched":                  "已切换至: %s",
	"switch_failed":             "切换失败: %s",
	"switch_expired_hint":       "账号 %s 信息已过期，建议执行: atomcode-switch-cli update",
	"auto_switched":             "自动切换至账号: %s",
	"account_deleted":           "账号已删除",
	"delete_canceled":           "已取消删除",
	"account_not_found":         "未找到账号: %s",
	"auth_not_found":            "未找到当前的 auth.toml 文件",
	"user_unmatched":            "识别到登录用户「%s」，但账号列表中无匹配账号，已中止更新以防串号",
	"no_fields":                 "未能识别出任何账号信息（套餐/用量/剩余天数/重置时间），请确认是完整的 /login 输出",
	"no_active":                 "没有激活的账号，请先执行 import",
	"active_not_found":          "激活账号不在账号列表中",
	"updated_from_paste":        "已更新账号: %s",
	"updated_manual":            "账号信息已更新",
	"fetching":                  "正在获取登录信息...",
	"fetch_timeout":             "获取登录信息超时，请重试或手动检查 atomcode login",
	"fetch_no_output":           "未获取到输出，请重试或手动检查 atomcode login",
	"fetch_spawn_failed":        "无法执行 atomcode login: %s",
	"config_load_failed":        "配置加载失败: %s（原文件已备份为 .bak，已加载默认配置）",
	"delete_confirm":            "确定要删除账号「%s」吗？此操作不可撤销",
	"exported":                  "导出成功: %s（注意：文件包含明文 token）",
	"export_failed":             "导出失败: %s",
	"imported":                  "已导入 %d 个账号",
	"imported_hint":             "导入成功: %s（信息未知，请执行 update 运行 /login）",
	"imported_existing":         "已同步账号: %s",
	"import_file_bad":           "账号文件读取失败: %s",
	"import_file_invalid":       "账号文件格式无效: %s",
	"config_saved":              "配置已保存",
	"config_save_failed":        "配置保存失败: %s",
	"list_empty":                "暂无账号，请先执行 import",
	"invalid_threshold":         "自动切换阈值必须在 50 到 100 之间",
	"invalid_interval":          "检查间隔必须大于 0",
	"invalid_selection":         "请指定账号序号、ID、名称或用户名",
	"invalid_language":          "语言只支持 zh_cn 或 en_us",
	"invalid_sort_mode":         "排序方式只支持 None、ResetTime 或 WindowRemaining",
	"invalid_data_dir":          "数据目录不能为空",
	"sync_failed":               "同步当前 auth.toml 失败: %s",
	"daemon_started":            "自动切换守护进程已启动，检查间隔 %s",
	"daemon_stopped":            "自动切换守护进程已停止",
	"config_show_title":         "当前配置",
	"config_field_data_dir":     "数据目录",
	"config_field_atomcode_dir": "AtomCode 目录",
	"config_field_language":     "语言",
	"config_field_sort_mode":    "排序方式",
	"config_field_auto_switch":  "自动切换",
	"config_field_threshold":    "用量阈值",
	"config_field_auto_update":  "切换过期账号时自动更新",
	"config_field_accounts":     "账号数量",
	"config_field_active":       "激活账号",
	"config_usage_set":          "用法: config set <key> <value>",
	"config_unknown_key":        "未知配置项: %s",
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
	"menu_yes":                  "确认",
	"menu_no":                   "取消",
	"menu_on":                   "开",
	"menu_off":                  "关",
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
}

var en = map[string]string{
	"plan_unknown":              "Unknown plan",
	"switched":                  "Switched to: %s",
	"switch_failed":             "Switch failed: %s",
	"switch_expired_hint":       "Account %s is expired, consider: atomcode-switch-cli update",
	"auto_switched":             "Auto-switched to: %s",
	"account_deleted":           "Account deleted",
	"delete_canceled":           "Delete canceled",
	"account_not_found":         "Account not found: %s",
	"auth_not_found":            "auth.toml not found",
	"user_unmatched":            "Detected login user %q but no matching account; update aborted",
	"no_fields":                 "No account info recognized (plan/usage/remaining/reset); is this complete /login output?",
	"no_active":                 "No active account, run import first",
	"active_not_found":          "Active account is not in the account list",
	"updated_from_paste":        "Account updated: %s",
	"updated_manual":            "Account info updated",
	"fetching":                  "Fetching login info...",
	"fetch_timeout":             "Timed out fetching login info",
	"fetch_no_output":           "No output received",
	"fetch_spawn_failed":        "Unable to run atomcode login: %s",
	"config_load_failed":        "Config load failed: %s (backed up to .bak, defaults loaded)",
	"delete_confirm":            "Delete account %q? This cannot be undone",
	"exported":                  "Exported: %s (warning: contains plaintext tokens)",
	"export_failed":             "Export failed: %s",
	"imported":                  "Imported %d accounts",
	"imported_hint":             "Imported: %s (info unknown, run update to execute /login)",
	"imported_existing":         "Synced account: %s",
	"import_file_bad":           "Failed to read accounts file: %s",
	"import_file_invalid":       "Invalid accounts file: %s",
	"config_saved":              "Config saved",
	"config_save_failed":        "Failed to save config: %s",
	"list_empty":                "No accounts yet, run import first",
	"invalid_threshold":         "Auto-switch threshold must be between 50 and 100",
	"invalid_interval":          "Check interval must be greater than 0",
	"invalid_selection":         "Specify an account index, ID, name or username",
	"invalid_language":          "Language must be zh_cn or en_us",
	"invalid_sort_mode":         "Sort mode must be None, ResetTime or WindowRemaining",
	"invalid_data_dir":          "Data directory cannot be empty",
	"sync_failed":               "Failed to sync current auth.toml: %s",
	"daemon_started":            "Auto-switch daemon started, interval %s",
	"daemon_stopped":            "Auto-switch daemon stopped",
	"config_show_title":         "Current configuration",
	"config_field_data_dir":     "Data directory",
	"config_field_atomcode_dir": "AtomCode directory",
	"config_field_language":     "Language",
	"config_field_sort_mode":    "Sort mode",
	"config_field_auto_switch":  "Auto switch",
	"config_field_threshold":    "Usage threshold",
	"config_field_auto_update":  "Auto-update on switching to expired account",
	"config_field_accounts":     "Account count",
	"config_field_active":       "Active account",
	"config_usage_set":          "Usage: config set <key> <value>",
	"config_unknown_key":        "Unknown config key: %s",
	"menu_title":                "AtomCode Account Manager",
	"menu_item_list":            "View accounts (%d)",
	"menu_item_switch":          "Switch account",
	"menu_item_update":          "Update account info (run /login)",
	"menu_item_sync":            "Sync current auth.toml",
	"menu_item_auto_switch":     "Check auto-switch now (currently: %s, threshold %.0f%%)",
	"menu_item_delete":          "Delete account",
	"menu_item_export":          "Export accounts",
	"menu_item_import_file":     "Import accounts from file",
	"menu_item_settings":        "Settings (active: %s)",
	"menu_item_quit":            "Quit",
	"menu_choose_hint":          "↑/↓ navigate, Enter confirm, q cancel",
	"menu_press_enter":          "Press Enter to return...",
	"menu_requires_tty":         "Interactive menu requires a TTY; use subcommands or run menu in a terminal",
	"menu_confirm_title":        "Please confirm",
	"menu_yes":                  "Yes",
	"menu_no":                   "No",
	"menu_on":                   "On",
	"menu_off":                  "Off",
	"menu_back":                 "Back to main menu",
	"menu_switch_title":         "Select account to switch to",
	"menu_delete_title":         "Select account to delete",
	"menu_export_prompt":        "Export file name",
	"menu_import_prompt":        "Enter YAML file path",
	"menu_settings_title":       "Settings",
	"menu_settings_language":    "Language (current: %s)",
	"menu_settings_sort":        "Sort mode (current: %s)",
	"menu_settings_auto_switch": "Auto switch (current: %s)",
	"menu_settings_threshold":   "Auto-switch threshold (current: %.0f%%)",
	"menu_threshold_prompt":     "Enter new threshold (50-100, Enter to cancel)",
	"auto_switch_none":          "No auto-switch needed",
}
