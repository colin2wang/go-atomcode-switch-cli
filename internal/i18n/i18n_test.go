package i18n

import (
	"strings"
	"testing"
)

func TestLanguageKeysAligned(t *testing.T) {
	if len(zh) != len(en) {
		t.Fatalf("zh keys = %d, en keys = %d", len(zh), len(en))
	}
	for key := range zh {
		if _, ok := en[key]; !ok {
			t.Errorf("en missing key %q", key)
		}
	}
	for key := range en {
		if _, ok := zh[key]; !ok {
			t.Errorf("zh missing key %q", key)
		}
	}
}

func TestCanonicalLanguageAliases(t *testing.T) {
	cases := map[string]string{
		"zh_cn":   zhCN,
		"ZH":      zhCN,
		"en":      enUS,
		"EN_US":   enUS,
		"en-us":   enUS,
		"English": enUS,
		"junk":    zhCN,
	}
	for input, want := range cases {
		if got := canonical(input); got != want {
			t.Errorf("canonical(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestInitPrefersConfigUnlessEnvOverride(t *testing.T) {
	original := lang
	t.Cleanup(func() { lang = original })
	t.Setenv("ATOMCODE_SWITCH_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "")

	Init("en-us")
	if Current() != enUS {
		t.Fatalf("Current() = %q, want en_us", Current())
	}

	t.Setenv("ATOMCODE_SWITCH_LANG", "zh_cn")
	Init("en_us")
	if Current() != zhCN {
		t.Fatalf("Current() = %q, want zh_cn after env override", Current())
	}
}

func TestUnknownKeyFallsBackAndFormatting(t *testing.T) {
	original := lang
	t.Cleanup(func() { lang = original })
	lang = zhCN
	if got := T("missing_key"); got != "missing_key" {
		t.Fatalf("T(missing_key) = %q", got)
	}
	got := T("switched", "Ada")
	if !strings.Contains(got, "Ada") {
		t.Fatalf("T(switched) = %q, want formatted name", got)
	}
}
