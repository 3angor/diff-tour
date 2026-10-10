// Package text は、端末に描く行を「スタイル付きの文字列の並び」として扱う。
// 日本語は 1 文字で 2 マス使うので、幅はすべて表示幅で数える。
package text

import (
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Span は同じスタイルで描く文字列。Style は SGR のパラメータ（例: "1;36"）。
type Span struct {
	Text  string
	Style string
}

// Line は端末の 1 行分。
type Line []Span

// 罫線などの「幅が曖昧な文字」は 1 マスとして数える。
// go-runewidth は LANG が ja_JP だと 2 マスとみなすが、多くの端末は 1 マスで描くため。
var cond = func() *runewidth.Condition {
	c := runewidth.NewCondition()
	c.EastAsianWidth = false
	return c
}()

func RuneWidth(r rune) int { return cond.RuneWidth(r) }

func Width(s string) int { return cond.StringWidth(s) }

func (l Line) Width() int {
	w := 0
	for _, s := range l {
		w += Width(s.Text)
	}
	return w
}

// Join は空でないスタイルを ";" でつなぐ。
func Join(styles ...string) string {
	parts := make([]string, 0, len(styles))
	for _, s := range styles {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ";")
}

// Append は直前と同じスタイルならくっつけて、違えば新しい Span を足す。
func Append(l Line, s, style string) Line {
	if s == "" {
		return l
	}
	if n := len(l); n > 0 && l[n-1].Style == style {
		l[n-1].Text += s
		return l
	}
	return append(l, Span{s, style})
}

func Concat(lines ...Line) Line {
	var out Line
	for _, l := range lines {
		for _, s := range l {
			out = Append(out, s.Text, s.Style)
		}
	}
	return out
}

// Map はすべての Span のスタイルを書き換えた行を返す。
func (l Line) Map(f func(style string) string) Line {
	out := make(Line, len(l))
	for i, s := range l {
		out[i] = Span{s.Text, f(s.Style)}
	}
	return out
}

// Fit は幅ちょうどに切り詰める（はみ出すなら末尾を … にする）か、padStyle の空白で埋める。
func Fit(l Line, width int, padStyle string) Line {
	if width <= 0 {
		return nil
	}
	overflow := l.Width() > width
	limit := width
	if overflow {
		limit = width - 1
	}
	var out Line
	w := 0
outer:
	for _, s := range l {
		for _, r := range s.Text {
			rw := RuneWidth(r)
			if w+rw > limit {
				break outer
			}
			out = Append(out, string(r), s.Style)
			w += rw
		}
	}
	if overflow {
		out = Append(out, "…", Join(padStyle, "2"))
		w++
	}
	if w < width {
		out = Append(out, strings.Repeat(" ", width-w), padStyle)
	}
	return out
}

// TrimRight は行末の空白を取り除く。
func TrimRight(l Line) Line {
	out := append(Line(nil), l...)
	for len(out) > 0 {
		last := &out[len(out)-1]
		last.Text = strings.TrimRight(last.Text, " ")
		if last.Text != "" {
			break
		}
		out = out[:len(out)-1]
	}
	return out
}

func ANSI(l Line, plain bool) string {
	var b strings.Builder
	for _, s := range l {
		if s.Style == "" || plain {
			b.WriteString(s.Text)
			continue
		}
		b.WriteString("\x1b[" + s.Style + "m" + s.Text + "\x1b[0m")
	}
	return b.String()
}

var (
	boldRe = regexp.MustCompile(`\*\*.+?\*\*`)
	codeRe = regexp.MustCompile("`[^`]+`")
)

// split は re に合う部分と合わない部分を、順番どおりに返す。
func split(s string, re *regexp.Regexp) (parts []string, matched []bool) {
	last := 0
	for _, loc := range re.FindAllStringIndex(s, -1) {
		if loc[0] > last {
			parts, matched = append(parts, s[last:loc[0]]), append(matched, false)
		}
		parts, matched = append(parts, s[loc[0]:loc[1]]), append(matched, true)
		last = loc[1]
	}
	if last < len(s) {
		parts, matched = append(parts, s[last:]), append(matched, false)
	}
	return parts, matched
}

// ParseInline は **強調** と `code` だけを解釈する。強調の中に code が入ってもよい。
func ParseInline(s, base string) []Span {
	var spans []Span
	parts, bold := split(s, boldRe)
	for i, part := range parts {
		style := base
		if bold[i] {
			part = part[2 : len(part)-2]
			style = Join(base, "1")
		}
		pieces, code := split(part, codeRe)
		for j, piece := range pieces {
			if code[j] {
				spans = append(spans, Span{piece[1 : len(piece)-1], Join(style, "36")})
			} else {
				spans = append(spans, Span{piece, style})
			}
		}
	}
	return spans
}

// 行頭に来てはいけない文字。直前の単位にくっつけて、一緒に折り返す。
const noLineStart = "、。，．）」』】〕！？：；・ー…’”)]}.,!?:;"

type unit struct {
	text  string
	style string
}

func units(spans []Span) []unit {
	var out []unit
	for _, s := range spans {
		var word strings.Builder
		flush := func() {
			if word.Len() > 0 {
				out = append(out, unit{word.String(), s.Style})
				word.Reset()
			}
		}
		for _, r := range s.Text {
			switch {
			case r == '\n' || r == ' ':
				flush()
				out = append(out, unit{string(r), s.Style})
			case RuneWidth(r) == 2:
				flush()
				out = append(out, unit{string(r), s.Style})
			default:
				word.WriteRune(r)
			}
		}
		flush()
	}
	var merged []unit
	for _, u := range out {
		if n := len(merged); n > 0 {
			prev := &merged[n-1]
			first := []rune(u.text)[0]
			if prev.text != "\n" && prev.text != " " && prev.style == u.style && strings.ContainsRune(noLineStart, first) {
				prev.text += u.text
				continue
			}
		}
		merged = append(merged, u)
	}
	return merged
}

// Wrap は幅に収まるように折り返す。英単語は途中で切らず、日本語はどこでも折り返せる。
func Wrap(spans []Span, width int) []Line {
	var lines []Line
	var cur Line
	w := 0
	newline := func() {
		lines = append(lines, TrimRight(cur))
		cur, w = nil, 0
	}
	for _, u := range units(spans) {
		if u.text == "\n" {
			newline()
			continue
		}
		uw := Width(u.text)
		if u.text == " " && (w == 0 || w+1 > width) {
			continue
		}
		if w+uw > width && w > 0 {
			newline()
		}
		if uw > width {
			for _, r := range u.text {
				rw := RuneWidth(r)
				if w+rw > width {
					newline()
				}
				cur = Append(cur, string(r), u.style)
				w += rw
			}
			continue
		}
		cur = Append(cur, u.text, u.style)
		w += uw
	}
	return append(lines, TrimRight(cur))
}
