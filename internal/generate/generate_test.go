package generate

import (
	"strings"
	"testing"

	"github.com/DaiAoki/tour/internal/i18n"
)

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                          `{"a":1}`,
		"```json\n{\"a\":1}\n```":          `{"a":1}`,
		"notes first\n\n{\"a\":{\"b\":2}}": `{"a":{"b":2}}`,
	}
	for in, want := range cases {
		if got := string(extractJSON(in)); got != want {
			t.Errorf("extractJSON(%q) = %q, want %q", in, got, want)
		}
	}
	if extractJSON("no json here") != nil {
		t.Error("expected nil")
	}
}

func TestParseReportsProblems(t *testing.T) {
	req := Request{Base: "main", Head: "feature"}
	if _, problems := parse(req, "not json"); len(problems) != 1 {
		t.Errorf("problems = %v", problems)
	}
	// 形式にない項目は、書き間違いとして直させる
	if _, problems := parse(req, `{"version":1,"title":"x","base":"main","head":"feature","chapters":[],"extra":1}`); len(problems) != 1 || !strings.Contains(problems[0], "extra") {
		t.Errorf("problems = %v", problems)
	}
}

func TestBuildPrompt(t *testing.T) {
	for _, lang := range i18n.Langs {
		p, err := buildPrompt(Request{Base: "main", Head: "feature/x", Lang: lang})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(p, languageRules[lang]) {
			t.Errorf("%s: language rule missing", lang)
		}
		if strings.Contains(p, "{{") {
			t.Errorf("%s: template left unexpanded", lang)
		}
		if !strings.Contains(p, "`feature/x`") {
			t.Errorf("%s: target missing", lang)
		}
	}
}
