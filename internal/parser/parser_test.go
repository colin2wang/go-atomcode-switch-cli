package parser

import (
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 11, 18, 10, 30, 0, 0, time.Local)
}

func TestParseChineseLoginOutput(t *testing.T) {
	text := "已登录\n套餐：CodingPlan Lite · expires 2026-12-31\n剩余 19 天\n当前时间窗口用量约 12%\n重置于 21:04 (in 10h 34m)"
	f := Parse(text, fixedNow())
	if !f.HasPlan || f.PlanName != "CodingPlan Lite" {
		t.Fatalf("plan = %q, %v", f.PlanName, f.HasPlan)
	}
	if !f.HasDays || f.RemainingDays != 19 {
		t.Fatalf("days = %d, %v", f.RemainingDays, f.HasDays)
	}
	if !f.HasUsage || f.UsagePercent != 12 {
		t.Fatalf("usage = %v, %v", f.UsagePercent, f.HasUsage)
	}
	if !f.HasReset || f.ResetTime != "2026-11-18 21:04" {
		t.Fatalf("reset = %q, %v", f.ResetTime, f.HasReset)
	}
}

func TestParseEnglishLoginOutput(t *testing.T) {
	text := "Logged in\nPlan: Pro Plan · expires soon\n19d / 30d remaining\nUsage: 45%\nresets 21:04 (in 10h)"
	f := Parse(text, fixedNow())
	if !f.HasPlan || f.PlanName != "Pro Plan" {
		t.Fatalf("plan = %q, %v", f.PlanName, f.HasPlan)
	}
	if !f.HasDays || f.RemainingDays != 19 {
		t.Fatalf("days = %d, %v", f.RemainingDays, f.HasDays)
	}
	if !f.HasUsage || f.UsagePercent != 45 {
		t.Fatalf("usage = %v, %v", f.UsagePercent, f.HasUsage)
	}
	if !f.HasReset || f.ResetTime != "2026-11-18 21:04" {
		t.Fatalf("reset = %q, %v", f.ResetTime, f.HasReset)
	}
}

func TestParsePureEnglishUsage(t *testing.T) {
	f := Parse("Usage: 12%", fixedNow())
	if !f.HasUsage || f.UsagePercent != 12 {
		t.Fatalf("usage = %v, %v", f.UsagePercent, f.HasUsage)
	}
}

func TestGarbageResetAndOutOfRangeUsageRejected(t *testing.T) {
	f := Parse("重置于 garbage text", fixedNow())
	if !f.Empty() {
		t.Fatalf("expected empty fields, got %+v", f)
	}
	f = Parse("用量约 150%", fixedNow())
	if f.HasUsage {
		t.Fatalf("out of range usage accepted: %v", f.UsagePercent)
	}
	f = Parse("用量约 0%", fixedNow())
	if !f.HasUsage || f.UsagePercent != 0 {
		t.Fatalf("zero usage rejected: %+v", f)
	}
}

func TestParseRealLoginOutputWithSpaceSeparators(t *testing.T) {
	text := "  AtomCode CodingPlan 配置：\n\n  ✓ already logged in as JW(hzjwang)\n  ✓ CodingPlan Lite-体验版 生效\n  ✓ 已添加 1 个账号 · 2 个模型：\n      • AtomGit  ·  qwen3.8-27b  （默认）\n      • AtomGit  ·  glm5.3-flash\n  ✓ 视觉预处理器 → AtomGit · qwen3.8-27b  （自动检测）\n  ✓ CodingPlan 状态：\n      套餐：CodingPlan Lite-体验版    到期时间 2026-11-25（剩余 5d / 共 7d）\n      用量：当前时间窗口用量约 23%    重置于 14:17（9m 40s 后）\n\n  Starting AtomCode...\n"
	f := Parse(text, fixedNow())
	if !f.HasPlan || f.PlanName != "CodingPlan Lite-体验版" {
		t.Fatalf("plan = %q, %v", f.PlanName, f.HasPlan)
	}
	if !f.HasDays || f.RemainingDays != 5 {
		t.Fatalf("days = %d, %v", f.RemainingDays, f.HasDays)
	}
	if !f.HasUsage || f.UsagePercent != 23 {
		t.Fatalf("usage = %v, %v", f.UsagePercent, f.HasUsage)
	}
	if !f.HasReset || f.ResetTime != "2026-11-18 14:17" {
		t.Fatalf("reset = %q, %v", f.ResetTime, f.HasReset)
	}
}
