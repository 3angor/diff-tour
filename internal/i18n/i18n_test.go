package i18n

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		all, lang string
		want      Lang
	}{
		{"", "ja_JP.UTF-8", JA},
		{"", "en_US.UTF-8", EN},
		{"", "fr_FR.UTF-8", EN}, // 対応していない言語は英語
		{"C", "ja_JP.UTF-8", JA},
		{"en_US.UTF-8", "ja_JP.UTF-8", EN}, // LC_ALL が優先
		{"", "", EN},
	}
	for _, c := range cases {
		t.Setenv("LC_ALL", c.all)
		t.Setenv("LC_MESSAGES", "")
		t.Setenv("LANG", c.lang)
		if got, _ := Detect(); got != c.want {
			t.Errorf("LC_ALL=%q LANG=%q: got %s, want %s", c.all, c.lang, got, c.want)
		}
	}
}

func TestEveryMessageHasJapanese(t *testing.T) {
	for k := range messages[EN] {
		if _, ok := messages[JA][k]; !ok {
			t.Errorf("missing ja message: %s", k)
		}
	}
}
