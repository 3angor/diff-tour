package schema

import "testing"

func TestColumnDefs(t *testing.T) {
	defs := columnDefs(`CREATE TABLE t (
  id TEXT PRIMARY KEY,
  -- comment, with a comma
  status TEXT NOT NULL CHECK (status IN ('A', 'B')),
  n INTEGER NOT NULL DEFAULT 0
, extra TEXT REFERENCES other (id))`)
	want := map[string]string{
		"id":     "TEXT PRIMARY KEY",
		"status": "TEXT NOT NULL CHECK (status IN ('A', 'B'))",
		"n":      "INTEGER NOT NULL DEFAULT 0",
		"extra":  "TEXT REFERENCES other (id)",
	}
	for k, v := range want {
		if defs[k] != v {
			t.Errorf("%s = %q, want %q", k, defs[k], v)
		}
	}
}

func TestDiffTable(t *testing.T) {
	before := &Table{Name: "t", Columns: []Column{{"id", "TEXT", ""}, {"old", "TEXT", ""}, {"n", "INTEGER", ""}}}
	after := &Table{Name: "t", Columns: []Column{{"id", "TEXT", ""}, {"n", "INTEGER", "NOT NULL"}, {"new", "TEXT", ""}}}
	d := DiffTable(before, after)
	if d.Change != Changed {
		t.Errorf("table change = %v", d.Change)
	}
	got := map[string]Change{}
	for _, c := range d.Columns {
		got[c.Name] = c.Change
	}
	want := map[string]Change{"id": Same, "n": Changed, "new": Added, "old": Removed}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if d := DiffTable(nil, after); d.Change != Added {
		t.Errorf("new table change = %v", d.Change)
	}
}
