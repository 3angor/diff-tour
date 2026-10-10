package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

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

// ソースコードで使っている文言のキーが、すべて英語と日本語の表にあること（キーがそのまま画面に出るのを防ぐ）
func TestUsedKeysExist(t *testing.T) {
	re := regexp.MustCompile(`(?:i18n\.T|statusText)\("([a-zA-Z.]+)"|"(setup\.[a-zA-Z]+)(?:[:|"])`)
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			key := m[1] + m[2]
			for _, lang := range Langs {
				if _, ok := messages[lang][key]; !ok {
					t.Errorf("%s: %s message missing: %s", path, lang, key)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
