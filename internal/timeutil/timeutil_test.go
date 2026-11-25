package timeutil

import (
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 11, 18, 10, 30, 0, 0, time.Local)
}

func TestParseHHMMStrict(t *testing.T) {
	valid := map[string][2]int{
		"09:05":       {9, 5},
		"23:59":       {23, 59},
		" 23:59 rest": {23, 59},
	}
	for raw, want := range valid {
		h, m, ok := ParseHHMM(raw)
		if !ok || h != want[0] || m != want[1] {
			t.Errorf("ParseHHMM(%q) = (%d,%d,%v), want (%d,%d,true)", raw, h, m, ok, want[0], want[1])
		}
	}
	for _, raw := range []string{"24:00", "12:60", "9:05", "-1:05", "+1:05", " 1:05", "中:05", "ab:cd", "1:5"} {
		if _, _, ok := ParseHHMM(raw); ok {
			t.Errorf("ParseHHMM(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestParseResetDatetime(t *testing.T) {
	now := fixedNow()

	got, ok := ParseResetDatetime("2026-11-18 12:00", now)
	if !ok || got.Format(LayoutFull) != "2026-11-18 12:00" {
		t.Fatalf("new format = %s, %v", got, ok)
	}

	got, ok = ParseResetDatetime("09:00", now)
	if !ok || got.Format(LayoutFull) != "2026-11-19 09:00" {
		t.Fatalf("past legacy = %s, %v", got, ok)
	}

	got, ok = ParseResetDatetime("11:00", now)
	if !ok || got.Format(LayoutFull) != "2026-11-18 11:00" {
		t.Fatalf("future legacy = %s, %v", got, ok)
	}

	got, ok = ParseResetDatetime("10:30", now)
	if !ok || got.Format(LayoutFull) != "2026-11-19 10:30" {
		t.Fatalf("equal legacy = %s, %v, want rollover", got, ok)
	}

	for _, raw := range []string{"", "not a time", "99:99"} {
		if _, ok := ParseResetDatetime(raw, now); ok {
			t.Errorf("ParseResetDatetime(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestCombineResetDatetime(t *testing.T) {
	now := fixedNow()
	if got, ok := CombineResetDatetime("09:00", now); !ok || got != "2026-11-19 09:00" {
		t.Fatalf("combine past = %q, %v", got, ok)
	}
	if got, ok := CombineResetDatetime("11:00", now); !ok || got != "2026-11-18 11:00" {
		t.Fatalf("combine future = %q, %v", got, ok)
	}
	if _, ok := CombineResetDatetime("99:99", now); ok {
		t.Fatal("combine 99:99 unexpectedly succeeded")
	}
}

func TestExpiryHelpers(t *testing.T) {
	now := fixedNow()
	if !IsExpired("2026-11-18 09:00", now) {
		t.Fatal("expected expired account")
	}
	if IsExpired("2026-11-18 12:00", now) {
		t.Fatal("unexpected expired account")
	}
	if !IsExpiringSoon("2026-11-18 11:00", now) {
		t.Fatal("expected expiring soon account")
	}
	if IsExpiringSoon("2026-11-18 12:00", now) {
		t.Fatal("unexpected expiring soon account")
	}
}
