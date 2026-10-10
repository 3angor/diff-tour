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
