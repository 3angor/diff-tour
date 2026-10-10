// Package tour defines the tour data format.
package tour

import (
	"encoding/json"
	"fmt"
	"os"
)

// Version is the tour format version this tool can read.
const Version = 1

// Basis is the grounds for an explanation.
type Basis string

const (
	Fact     Basis = "fact"     // verified (backed by code, tests or commit messages)
	Inferred Basis = "inferred" // can be read from the structure of the code
	Opinion  Basis = "opinion"  // a senior's view (general wisdom, not verified in this repository)
)

type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
	// Which revision of the code: "base" or "head" (default).
	At string `json:"at,omitempty"`
}

type CodeRef struct {
	File string `json:"file"`
	At   string `json:"at,omitempty"`
	// Lines to highlight [start, end].
	Lines [2]int `json:"lines"`
	// Range to display. Defaults to 2 lines around the highlighted lines.
	Show *[2]int `json:"show,omitempty"`
	// A side note such as "unchanged".
	Note string `json:"note,omitempty"`
}

type DiffRef struct {
	File string `json:"file"`
	// Indexes of the hunks to show (0-based). Defaults to all.
	Hunks []int `json:"hunks,omitempty"`
}

// Snippet is code that is not in the repository. Label must always say so.
type Snippet struct {
	Label string `json:"label"`
	Lang  string `json:"lang,omitempty"`
	Text  string `json:"text"`
}

type SchemaRef struct {
	// Directory holding the migration SQL (applied in file-name order).
	Migrations string `json:"migrations"`
	// Tables to show. Defaults to every table that changed.
	Tables []string `json:"tables,omitempty"`
	// Columns to highlight ("table.column").
	Focus []string `json:"focus,omitempty"`
}

type Rows struct {
	// A short tag showing where the table came from. Defaults to "example" (data made up for the explanation).
	Tag   string     `json:"tag,omitempty"`
	Label string     `json:"label"`
	Head  []string   `json:"head"`
	Data  [][]string `json:"data"`
	// Columns added by this change.
	Added []string `json:"added,omitempty"`
}

type Table struct {
	Head []string   `json:"head"`
	Rows [][]string `json:"rows"`
}

// Block is one part of a step. It holds exactly one kind of item.
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

// IsVisual reports whether the block is laid out on the left, like code or data.
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
	// A step that shows only the question. The answer goes in the next step.
	Question bool `json:"question,omitempty"`
	// Where `o` opens. Defaults to the first code block.
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

// Validate checks for format errors. Whether the referenced files and lines exist is checked at display time.
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
