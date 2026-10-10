// Package highlight は chroma でコードを色分けし、行ごとの text.Line にする。
package highlight

import (
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/DaiAoki/tour/internal/text"
)

const (
	styleComment = "90"
	styleKeyword = "35"
	styleType    = "36"
	styleString  = "32"
	styleNumber  = "33"
)

func lexerFor(filename, lang, source string) chroma.Lexer {
	var l chroma.Lexer
	if lang != "" {
		l = lexers.Get(lang)
	}
	if l == nil && filename != "" {
		l = lexers.Match(filename)
	}
	if l == nil {
		l = lexers.Analyse(source)
	}
	if l == nil {
		l = lexers.Fallback
	}
	return chroma.Coalesce(l)
}

func styleFor(t chroma.Token) string {
	switch {
	case t.Type.InCategory(chroma.Comment):
		return styleComment
	case t.Type == chroma.KeywordType || t.Type == chroma.NameClass || t.Type == chroma.NameBuiltin:
		return styleType
	case t.Type.InCategory(chroma.Keyword):
		return styleKeyword
	case t.Type.InSubCategory(chroma.LiteralString):
		return styleString
	case t.Type.InSubCategory(chroma.LiteralNumber):
		return styleNumber
	case t.Type.InCategory(chroma.Name):
		// 大文字で始まる名前は型やクラスであることが多い
		if r := []rune(t.Value); len(r) > 0 && unicode.IsUpper(r[0]) {
			return styleType
		}
	}
	return ""
}

// Lines は src を色分けする。言語は lang、なければファイル名、それもなければ中身から推測する。
// 複数行のコメントなどが正しく色分けされるよう、表示しない行も含めてまとめて渡す。
func Lines(filename, lang string, src []string) []text.Line {
	for i, s := range src {
		src[i] = strings.ReplaceAll(s, "\t", "  ")
	}
	source := strings.Join(src, "\n")
	out := make([]text.Line, 1, len(src))
	it, err := lexerFor(filename, lang, source).Tokenise(nil, source)
	if err != nil {
		out = out[:0]
		for _, s := range src {
			out = append(out, text.Line{{Text: s}})
		}
		return out
	}
	for tok := it(); tok != chroma.EOF; tok = it() {
		style := styleFor(tok)
		for i, part := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				out = append(out, nil)
			}
			out[len(out)-1] = text.Append(out[len(out)-1], part, style)
		}
	}
	for len(out) < len(src) {
		out = append(out, nil)
	}
	return out[:len(src)]
}
