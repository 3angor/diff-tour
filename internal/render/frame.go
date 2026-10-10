// Package render は、ツアーの手順を端末 1 画面分の行にする。
package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DaiAoki/tour/internal/i18n"
	"github.com/DaiAoki/tour/internal/text"
	"github.com/DaiAoki/tour/internal/tour"
)

const (
	pad = 2
	// これ以上の幅があれば、コードと説明を左右に並べる
	sideBySideMinCols = 110
)

func stack(blocks []tour.Block, width int, ctx Ctx) []text.Line {
	var out []text.Line
	for i, b := range blocks {
		if i > 0 {
			out = append(out, nil)
		}
		out = append(out, Block(b, width, ctx)...)
	}
	return out
}

func spread(left, right text.Line, width int) text.Line {
	room := width - right.Width()
	if room < 8 {
		return text.Fit(left, width, "")
	}
	return text.Concat(text.Fit(left, room, ""), right)
}

// Location は `o` で開く場所。開けるものがなければ nil。
func Location(step tour.FlatStep, ctx Ctx) *tour.Location {
	if step.Open != nil {
		return step.Open
	}
	for _, b := range step.Blocks {
		switch {
		case b.Code != nil:
			return &tour.Location{File: b.Code.File, Line: b.Code.Lines[0], At: b.Code.At}
		case b.Diff != nil:
			picked, errMsg := pickHunks(ctx, b.Diff)
			if errMsg != "" {
				return &tour.Location{File: b.Diff.File, Line: 1}
			}
			// 最初に変わった行。削除だけの行には head での行番号がないので、直前の行の番号を使う
			line := 1
		find:
			for _, h := range picked {
				for _, l := range h.Lines {
					if l.NewNo > 0 {
						line = l.NewNo
					}
					if l.Kind != ' ' {
						break find
					}
				}
			}
			return &tour.Location{File: b.Diff.File, Line: line}
		case b.Schema != nil:
			old := map[string]bool{}
			for _, f := range ctx.Repo.ListFiles(ctx.Ref("base"), b.Schema.Migrations) {
				old[f] = true
			}
			var files, added []string
			for _, f := range ctx.Repo.ListFiles(ctx.Tour.Head, b.Schema.Migrations) {
				if strings.HasSuffix(f, ".sql") {
					files = append(files, f)
					if !old[f] {
						added = append(added, f)
					}
				}
			}
			sort.Strings(files)
			sort.Strings(added)
			// 今回追加したマイグレーションの最後のもの。なければ全体の最後のもの
			if len(added) > 0 {
				return &tour.Location{File: added[len(added)-1], Line: 1}
			}
			if len(files) > 0 {
				return &tour.Location{File: files[len(files)-1], Line: 1}
			}
		}
	}
	return nil
}

// Frame は 1 画面分の行と、スクロールできる最大の量を返す。
func Frame(ctx Ctx, steps []tour.FlatStep, index, scroll, cols, rows int, message string) ([]text.Line, int) {
	step := steps[index]
	inner := max(20, cols-pad*2)

	header := []text.Line{
		spread(
			text.Line{{Text: " tour ", Style: "30;46"}, {Text: " " + ctx.Tour.Title, Style: "1"}},
			text.Line{{Text: i18n.T("chapter", step.Chapter+1, step.ChapterTitle) + " ", Style: "36"}},
			cols,
		),
		{{Text: strings.Repeat("─", cols), Style: "90"}},
	}

	titleSpans := []text.Span{{Text: step.Title, Style: "1"}}
	if step.Question {
		titleSpans = append([]text.Span{{Text: " " + i18n.T("question") + " ", Style: "30;43"}, {Text: " "}}, titleSpans...)
	}
	title := text.Wrap(titleSpans, inner)
	var hint []text.Line
	if step.Question {
		hint = []text.Line{nil, {{Text: i18n.T("think"), Style: "2;33"}}}
	}

	// 左に置くのは、先頭から続くコードのブロック（なければ最初のコードのブロック）だけ。
	// それ以外は説明の流れを崩さないよう、元の順番のまま右に置く
	var visual, other []tour.Block
	leading := 0
	for leading < len(step.Blocks) && step.Blocks[leading].IsVisual() {
		leading++
	}
	if leading > 0 {
		visual, other = step.Blocks[:leading], step.Blocks[leading:]
	} else {
		taken := false
		for _, b := range step.Blocks {
			if !taken && b.IsVisual() {
				visual, taken = append(visual, b), true
			} else {
				other = append(other, b)
			}
		}
	}

	body := append(append([]text.Line{}, title...), nil)
	if cols >= sideBySideMinCols && len(visual) > 0 && len(other) > 0 {
		// コードが切れないよう、左はコードの実際の幅に合わせる（全体の 45〜65% の範囲）
		natural := 0
		for _, l := range stack(visual, 400, ctx) {
			natural = max(natural, text.TrimRight(l).Width())
		}
		leftW := max(inner*45/100, min(natural, inner*65/100))
		rightW := inner - leftW - 3
		left := stack(visual, leftW, ctx)
		right := append(stack(other, rightW, ctx), hint...)
		for i := range max(len(left), len(right)) {
			var l, r text.Line
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			body = append(body, text.Concat(text.Fit(l, leftW, ""), text.Line{{Text: " │ ", Style: "90"}}, text.Fit(r, rightW, "")))
		}
	} else {
		body = append(body, stack(step.Blocks, inner, ctx)...)
		body = append(body, hint...)
	}

	bodyH := max(1, rows-len(header)-2)
	maxScroll := max(0, len(body)-bodyH)
	top := min(scroll, maxScroll)
	visible := body[top:min(len(body), top+bodyH)]

	out := header
	for i := range bodyH {
		var l text.Line
		if i < len(visible) {
			l = visible[i]
		}
		out = append(out, text.Concat(text.Line{{Text: strings.Repeat(" ", pad)}}, text.Fit(l, inner, ""), text.Line{{Text: strings.Repeat(" ", max(0, cols-inner-pad))}}))
	}

	barW := max(10, cols-10)
	filled := ((index+1)*barW*2 + len(steps)) / (len(steps) * 2)
	out = append(out, text.Fit(text.Line{
		{Text: " "},
		{Text: strings.Repeat("━", filled), Style: "36"},
		{Text: strings.Repeat("─", barW-filled), Style: "90"},
		{Text: fmt.Sprintf(" %d/%d", index+1, len(steps)), Style: "90"},
	}, cols, ""))

	backStyle := ""
	if index == 0 {
		backStyle = "90"
	}
	keys := text.Line{{Text: " " + i18n.T("key.back"), Style: backStyle}, {Text: "   "}}
	if index == len(steps)-1 {
		keys = append(keys, text.Span{Text: i18n.T("key.last"), Style: "90"})
	} else {
		keys = append(keys, text.Span{Text: i18n.T("key.next"), Style: "1"})
	}
	if Location(step, ctx) != nil {
		keys = append(keys, text.Span{Text: "   " + i18n.T("key.open"), Style: "90"})
	}
	keys = append(keys, text.Span{Text: "   " + i18n.T("key.quit"), Style: "90"})

	var right text.Line
	switch {
	case message != "":
		right = text.Line{{Text: message + " ", Style: "33"}}
	case maxScroll > 0 && top < maxScroll:
		right = text.Line{{Text: i18n.T("scroll.more") + " ", Style: "33"}}
	case maxScroll > 0:
		right = text.Line{{Text: i18n.T("scroll.top") + " ", Style: "33"}}
	}
	out = append(out, spread(keys, right, cols))
	return out, maxScroll
}
