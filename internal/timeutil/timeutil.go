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

// CombineResetDatetime 把 HH:MM 组合为完整的 YYYY-MM-DD HH:MM。
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
