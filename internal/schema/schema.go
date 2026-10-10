// Package schema applies migrations to an in-memory SQLite database to extract the schema at a given revision.
// It only reads the repository's contents and writes nothing to files.
package schema

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/init-co-jp/diff-tour/internal/git"
)

type Column struct {
	Name string
	Type string
	// The definition after the type (NOT NULL, DEFAULT, CHECK, REFERENCES, etc.).
	Constraints string
}

type Index struct {
	Name    string
	Columns []string
	Unique  bool
}

type Table struct {
	Name    string
	Columns []Column
	Indexes []Index
}

type Schema map[string]*Table

// UnsafeError is a migration containing a statement that could create files outside the in-memory DB.
type UnsafeError struct{ File string }

func (e *UnsafeError) Error() string { return "unsafe statement in " + e.File }

// MigrationError reports that a migration could not be applied.
type MigrationError struct {
	File string
	Err  error
}

func (e *MigrationError) Error() string { return fmt.Sprintf("%s: %v", e.File, e.Err) }

var unsafeRe = regexp.MustCompile(`(?i)\b(ATTACH|VACUUM|load_extension)\b`)

func Load(repo *git.Repo, ref, dir string) (Schema, error) {
	var files []string
	for _, f := range repo.ListFiles(ref, dir) {
		if strings.HasSuffix(f, ".sql") {
			files = append(files, f)
		}
	}
	sort.Strings(files)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	// :memory: gives each connection its own DB, so pin the pool to a single connection.
	db.SetMaxOpenConns(1)

	for _, f := range files {
		src := strings.Join(repo.Show(ref, f), "\n")
		if unsafeRe.MatchString(src) {
			return nil, &UnsafeError{f}
		}
		if _, err := db.Exec(src); err != nil {
			return nil, &MigrationError{f, err}
		}
	}

	rows, err := db.Query("SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	type tableSQL struct{ name, sql string }
	var tables []tableSQL
	for rows.Next() {
		var t tableSQL
		if err := rows.Scan(&t.name, &t.sql); err != nil {
			rows.Close()
			return nil, err
		}
		tables = append(tables, t)
	}
	rows.Close()

	s := Schema{}
	for _, t := range tables {
		table := &Table{Name: t.name}
		defs := columnDefs(t.sql)
		cols, err := db.Query(fmt.Sprintf("SELECT name, type FROM pragma_table_info(%s)", quote(t.name)))
		if err != nil {
			return nil, err
		}
		for cols.Next() {
			var c Column
			if err := cols.Scan(&c.Name, &c.Type); err != nil {
				cols.Close()
				return nil, err
			}
			def := defs[c.Name]
			if len(def) >= len(c.Type) && strings.EqualFold(def[:len(c.Type)], c.Type) {
				def = strings.TrimSpace(def[len(c.Type):])
			}
			c.Constraints = def
			table.Columns = append(table.Columns, c)
		}
		cols.Close()

		idx, err := db.Query(fmt.Sprintf("SELECT name, \"unique\", origin FROM pragma_index_list(%s) ORDER BY name", quote(t.name)))
		if err != nil {
			return nil, err
		}
		var indexes []Index
		for idx.Next() {
			var i Index
			var unique int
			var origin string
			if err := idx.Scan(&i.Name, &unique, &origin); err != nil {
				idx.Close()
				return nil, err
			}
			// Skip indexes created automatically for primary keys; the column definitions already show them.
			if origin == "pk" {
				continue
			}
			i.Unique = unique == 1
			indexes = append(indexes, i)
		}
		idx.Close()
		for _, i := range indexes {
			ic, err := db.Query(fmt.Sprintf("SELECT name FROM pragma_index_info(%s) ORDER BY seqno", quote(i.Name)))
			if err != nil {
				return nil, err
			}
			for ic.Next() {
				var name string
				if err := ic.Scan(&name); err != nil {
					ic.Close()
					return nil, err
				}
				i.Columns = append(i.Columns, name)
			}
			ic.Close()
			table.Indexes = append(table.Indexes, i)
		}
		s[t.name] = table
	}
	return s, nil
}

func quote(name string) string {
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}

var (
	commentRe = regexp.MustCompile(`--[^\n]*`)
	spaceRe   = regexp.MustCompile(`\s+`)
	colRe     = regexp.MustCompile("^[\"`\\[]?(\\w+)[\"`\\]]?\\s*(.*)$")
)

// columnDefs splits a CREATE TABLE statement into per-column definitions (dropping comments and not splitting on commas inside parentheses).
// SQLite saves ALTER TABLE ADD COLUMN by appending to the stored CREATE TABLE statement, so added columns are picked up here too.
func columnDefs(sql string) map[string]string {
	start, end := strings.Index(sql, "("), strings.LastIndex(sql, ")")
	defs := map[string]string{}
	if start < 0 || end <= start {
		return defs
	}
	body := commentRe.ReplaceAllString(sql[start+1:end], "")
	var parts []string
	var cur strings.Builder
	depth := 0
	var quoteChar rune
	for _, r := range body {
		switch {
		case quoteChar != 0:
			if r == quoteChar {
				quoteChar = 0
			}
		case r == '\'' || r == '"':
			quoteChar = r
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ',' && depth == 0:
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	parts = append(parts, cur.String())
	for _, p := range parts {
		p = strings.TrimSpace(spaceRe.ReplaceAllString(p, " "))
		if m := colRe.FindStringSubmatch(p); m != nil {
			defs[m[1]] = m[2]
		}
	}
	return defs
}

type Change int

const (
	Same Change = iota
	Added
	Removed
	Changed
)

type ColumnDiff struct {
	Column
	Change Change
}

type IndexDiff struct {
	Index
	Change Change
}

type TableDiff struct {
	Name    string
	Change  Change
	Columns []ColumnDiff
	Indexes []IndexDiff
}

// DiffTable compares a table before and after the change. If either is nil, the table was added or removed.
func DiffTable(before, after *Table) TableDiff {
	d := TableDiff{}
	switch {
	case before == nil:
		d.Name, d.Change = after.Name, Added
	case after == nil:
		d.Name, d.Change = before.Name, Removed
	default:
		d.Name = after.Name
	}
	if after != nil {
		for _, c := range after.Columns {
			change := Same
			if before != nil {
				old := findColumn(before, c.Name)
				switch {
				case old == nil:
					change = Added
				case old.Type != c.Type || old.Constraints != c.Constraints:
					change = Changed
				}
			}
			d.Columns = append(d.Columns, ColumnDiff{c, change})
		}
		for _, i := range after.Indexes {
			change := Same
			if before != nil && findIndex(before, i.Name) == nil {
				change = Added
			}
			d.Indexes = append(d.Indexes, IndexDiff{i, change})
		}
	}
	if before != nil && after != nil {
		for _, c := range before.Columns {
			if findColumn(after, c.Name) == nil {
				d.Columns = append(d.Columns, ColumnDiff{c, Removed})
			}
		}
		for _, i := range before.Indexes {
			if findIndex(after, i.Name) == nil {
				d.Indexes = append(d.Indexes, IndexDiff{i, Removed})
			}
		}
		for _, c := range d.Columns {
			if c.Change != Same {
				d.Change = Changed
			}
		}
		for _, i := range d.Indexes {
			if i.Change != Same {
				d.Change = Changed
			}
		}
	}
	if before == nil && after != nil {
		for i := range d.Columns {
			d.Columns[i].Change = Same
		}
	}
	return d
}

func findColumn(t *Table, name string) *Column {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i]
		}
	}
	return nil
}

func findIndex(t *Table, name string) *Index {
	for i := range t.Indexes {
		if t.Indexes[i].Name == name {
			return &t.Indexes[i]
		}
	}
	return nil
}
