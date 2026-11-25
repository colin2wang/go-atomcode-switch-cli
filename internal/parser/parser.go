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
