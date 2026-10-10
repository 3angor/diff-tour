// Package tour はツアーのデータ形式を定める。
package tour

import (
	"encoding/json"
	"fmt"
	"os"
)

// Version は、このツールが読めるツアーの形式の版。
const Version = 1

// Basis は説明の根拠。
type Basis string

const (
	Fact     Basis = "fact"     // 確かめた（コード・テスト・コミットメッセージで裏付けがある）
	Inferred Basis = "inferred" // コードの構造から読み取れる
	Opinion  Basis = "opinion"  // シニアの見方（一般論。このリポジトリでは確かめていない）
)

type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
	// どの時点のコードか。"base" か "head"（既定）
	At string `json:"at,omitempty"`
}

type CodeRef struct {
	File string `json:"file"`
	At   string `json:"at,omitempty"`
	// 強調する行 [開始, 終了]
	Lines [2]int `json:"lines"`
	// 表示する範囲。省略時は強調する行の前後 2 行
	Show *[2]int `json:"show,omitempty"`
	// 「変更なし」などの補足
	Note string `json:"note,omitempty"`
}

type DiffRef struct {
	File string `json:"file"`
	// 見せるハンクの番号（0 始まり）。省略時はすべて
	Hunks []int `json:"hunks,omitempty"`
}

// Snippet はリポジトリにないコード。Label でそれを必ず示す。
type Snippet struct {
	Label string `json:"label"`
	Lang  string `json:"lang,omitempty"`
	Text  string `json:"text"`
}

type SchemaRef struct {
	// マイグレーションの SQL があるディレクトリ（ファイル名の順に当てる）
	Migrations string `json:"migrations"`
	// 見せるテーブル。省略時は変更のあったテーブルすべて
	Tables []string `json:"tables,omitempty"`
	// 強調する列（"テーブル.列"）
	Focus []string `json:"focus,omitempty"`
}

type Rows struct {
	// 表の出どころを示す短い札。省略時は「例」（説明用に作ったデータ）
	Tag   string     `json:"tag,omitempty"`
	Label string     `json:"label"`
	Head  []string   `json:"head"`
	Data  [][]string `json:"data"`
	// 今回の変更で増えた列
	Added []string `json:"added,omitempty"`
}

type Table struct {
	Head []string   `json:"head"`
	Rows [][]string `json:"rows"`
}

// Block は手順の中の 1 つの部品。ちょうど 1 種類の項目だけを持つ。
type Block struct {
	P       *string    `json:"p,omitempty"`
	Basis   Basis      `json:"basis,omitempty"`
	Quote   *string    `json:"quote,omitempty"`
	List    []string   `json:"list,omitempty"`
	Pre     *string    `json:"pre,omitempty"`
	Table   *Table     `json:"table,omitempty"`
	Code    *CodeRef   `json:"code,omitempty"`
	Diff    *DiffRef   `json:"diff,omitempty"`
	Snippet *Snippet   `json:"snippet,omitempty"`
	Schema  *SchemaRef `json:"schema,omitempty"`
	Rows    *Rows      `json:"rows,omitempty"`
	Target  bool       `json:"target,omitempty"`
}

// IsVisual は、コードやデータのように左側に並べるブロックかどうか。
func (b Block) IsVisual() bool {
	return b.Code != nil || b.Diff != nil || b.Snippet != nil || b.Schema != nil || b.Rows != nil
}

func (b Block) kinds() int {
	n := 0
	for _, set := range []bool{b.P != nil, b.Quote != nil, b.List != nil, b.Pre != nil, b.Table != nil, b.Code != nil,
		b.Diff != nil, b.Snippet != nil, b.Schema != nil, b.Rows != nil, b.Target} {
		if set {
			n++
		}
	}
	return n
}

type Step struct {
	Title string `json:"title"`
	// 問いだけを見せる手順。答えは次の手順に置く
	Question bool `json:"question,omitempty"`
	// `o` で開く場所。省略時は最初のコードのブロックから決める
	Open   *Location `json:"open,omitempty"`
	Blocks []Block   `json:"blocks"`
}

type Chapter struct {
	Title string `json:"title"`
	Steps []Step `json:"steps"`
}

type Tour struct {
	Version  int       `json:"version"`
	Title    string    `json:"title"`
	Base     string    `json:"base"`
	Head     string    `json:"head"`
	Chapters []Chapter `json:"chapters"`
}

type FlatStep struct {
	Step
	Chapter      int
	ChapterTitle string
}

func (t *Tour) Steps() []FlatStep {
	var out []FlatStep
	for i, c := range t.Chapters {
		for _, s := range c.Steps {
			out = append(out, FlatStep{s, i, c.Title})
		}
	}
	return out
}

func Load(path string) (*Tour, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Tour
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &t, nil
}

// Validate は形式の誤りを調べる。指しているファイルや行が実在するかは、表示するときに確かめる。
func (t *Tour) Validate() error {
	if t.Version != Version {
		return fmt.Errorf("unsupported version %d (expected %d)", t.Version, Version)
	}
	if t.Base == "" || t.Head == "" {
		return fmt.Errorf("base and head are required")
	}
	if len(t.Steps()) == 0 {
		return fmt.Errorf("no steps")
	}
	for ci, c := range t.Chapters {
		for si, s := range c.Steps {
			if s.Title == "" {
				return fmt.Errorf("chapter %d step %d: title is required", ci+1, si+1)
			}
			for bi, b := range s.Blocks {
				if b.kinds() != 1 {
					return fmt.Errorf("chapter %d step %d block %d: a block must have exactly one kind", ci+1, si+1, bi+1)
				}
				if b.Basis != "" && b.P == nil {
					return fmt.Errorf("chapter %d step %d block %d: basis is only for p", ci+1, si+1, bi+1)
				}
				switch b.Basis {
				case "", Fact, Inferred, Opinion:
				default:
					return fmt.Errorf("chapter %d step %d block %d: unknown basis %q", ci+1, si+1, bi+1, b.Basis)
				}
			}
		}
	}
	return nil
}
