package git

import "testing"

func TestParseDiff(t *testing.T) {
	hunks := ParseDiff(`diff --git a/x b/x
@@ -1,3 +1,3 @@ func f() {
 a
-b
+c
 d
@@ -10,1 +10,2 @@
 e
+f
`)
	if len(hunks) != 2 {
		t.Fatalf("hunks = %d", len(hunks))
	}
	if hunks[0].Header != "func f() {" {
		t.Errorf("header = %q", hunks[0].Header)
	}
	l := hunks[0].Lines
	if l[1].Kind != '-' || l[1].OldNo != 2 || l[2].Kind != '+' || l[2].NewNo != 2 || l[3].NewNo != 3 {
		t.Errorf("lines = %+v", l)
	}
	if hunks[1].Lines[1].NewNo != 11 {
		t.Errorf("second hunk = %+v", hunks[1].Lines)
	}
}

func TestParseFileStats(t *testing.T) {
	diff := `diff --git a/src/a.ts b/src/a.ts
index 1..2 100644
--- a/src/a.ts
+++ b/src/a.ts
@@ -1,2 +1,3 @@
 keep
-old
+new
+more
diff --git a/db/002.sql b/db/002.sql
new file mode 100644
--- /dev/null
+++ b/db/002.sql
@@ -0,0 +1 @@
+CREATE TABLE x (id INTEGER);
`
	got := ParseFileStats(diff)
	want := []FileStat{{File: "src/a.ts", Insertions: 2, Deletions: 1}, {File: "db/002.sql", Insertions: 1, New: true}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}
