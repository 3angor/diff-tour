package text

import (
	"strings"
	"testing"
)

func plain(lines []Line) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ANSI(l, true)
	}
	return out
}

func TestWidth(t *testing.T) {
	cases := map[string]int{"abc": 3, "日本語": 6, "─│▶★": 4, "a日b": 4}
	for s, want := range cases {
		if got := Width(s); got != want {
			t.Errorf("Width(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestFit(t *testing.T) {
	if got := ANSI(Fit(Line{{Text: "日本語です"}}, 7, ""), true); got != "日本語…" {
		t.Errorf("truncate: got %q", got)
	}
	if got := ANSI(Fit(Line{{Text: "ab"}}, 5, ""), true); got != "ab   " {
		t.Errorf("pad: got %q", got)
	}
	if w := Fit(Line{{Text: "日本語です"}}, 8, "").Width(); w != 8 {
		t.Errorf("width after fit = %d", w)
	}
}

func TestWrapKeepsEnglishWords(t *testing.T) {
	got := plain(Wrap([]Span{{Text: "hello wonderful world"}}, 11))
	if strings.Join(got, "|") != "hello|wonderful|world" {
		t.Errorf("got %q", got)
	}
}

func TestWrapJapaneseDoesNotStartLineWithPunctuation(t *testing.T) {
	got := plain(Wrap([]Span{{Text: "あいうえ。かきく"}}, 8))
	for _, l := range got[1:] {
		if strings.HasPrefix(l, "。") {
			t.Errorf("line starts with punctuation: %q", got)
		}
	}
}

func TestParseInline(t *testing.T) {
	spans := ParseInline("a **b `c` d** `e`", "")
	var styles []string
	for _, s := range spans {
		styles = append(styles, s.Text+"="+s.Style)
	}
	want := "a =|b =1|c=1;36| d=1| =|e=36"
	if got := strings.Join(styles, "|"); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
