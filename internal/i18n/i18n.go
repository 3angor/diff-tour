// Package i18n は画面の文言を言語ごとに持つ。ツアーの本文はツアーのデータの言語のまま表示する。
package i18n

import (
	"fmt"
	"os"
	"strings"
)

type Lang string

const (
	EN Lang = "en"
	JA Lang = "ja"
)

// Langs は対応している言語。選択肢の並び順でもある。
var Langs = []Lang{EN, JA}

func (l Lang) Name() string {
	switch l {
	case JA:
		return "日本語"
	default:
		return "English"
	}
}

func Parse(s string) (Lang, bool) {
	for _, l := range Langs {
		if strings.EqualFold(s, string(l)) {
			return l, true
		}
	}
	return EN, false
}

var current = EN

func Set(l Lang) { current = l }

func Current() Lang { return current }

// Detect は端末の言語設定（LC_ALL → LC_MESSAGES → LANG）から言語を選ぶ。
// 対応していなければ英語にする。どの変数から読んだかも返す。
func Detect() (Lang, string) {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := os.Getenv(key)
		if v == "" || v == "C" || v == "POSIX" {
			continue
		}
		code := strings.ToLower(strings.SplitN(strings.SplitN(v, ".", 2)[0], "_", 2)[0])
		l, ok := Parse(code)
		if !ok {
			l = EN
		}
		return l, key + "=" + v
	}
	return EN, ""
}

// T は現在の言語の文言を返す。その言語になければ英語を使う。
func T(key string, args ...any) string {
	m, ok := messages[current][key]
	if !ok {
		m, ok = messages[EN][key]
	}
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(m, args...)
	}
	return m
}

var messages = map[Lang]map[string]string{
	EN: {
		"key.back":    "← Back",
		"key.next":    "→ Next",
		"key.last":    "Last step",
		"key.open":    "o Open in editor",
		"key.quit":    "q Quit",
		"scroll.more": "▼ More  ↑↓",
		"scroll.top":  "▲ ↑↓",
		"chapter":     "Ch. %d  %s",
		"question":    "Question",
		"think":       "Think it over, then press → to see the answer",

		"basis.fact":     "Verified",
		"basis.inferred": "From the code",
		"basis.opinion":  "Senior view",

		"tag.example":        "Example",
		"diff.summary":       "diff  +%d −%d",
		"schema.title":       "Schema",
		"schema.desc":        "%s applied at %s and at %s",
		"schema.newTable":    "new table",
		"change.added":       "added",
		"change.removed":     "removed",
		"change.changed":     "changed",
		"change.same":        "unchanged",
		"schema.column":      "Column",
		"schema.type":        "Type",
		"schema.constraints": "Constraints",
		"schema.index":       "index",
		"schema.unique":      "unique index",
		"rows.added":         "Green columns were added by this change",
		"target.branched":    "  (branched from %s)",
		"target.summary":     "%d commits · %d files changed (+%d −%d)",

		"err.fileNotFound": "File not found: %s (%s)",
		"err.lineRange":    "Lines out of range: %s L%d–%d (the file has %d lines)",
		"err.noDiff":       "No diff found: %s",
		"err.noHunk":       "Hunk not found: %s %v",
		"err.noTable":      "Table not found: %s",
		"err.unsafeSQL":    "Not executed for safety: %s",
		"err.migration":    "Cannot apply migration: %s (%v)",
		"err.schema":       "Cannot read the schema: %v",

		"editor.copy": "Opened a read-only copy: the working tree differs from %s",
		"editor.fail": "Cannot start the editor: %v",
		"needTTY":     "tour needs to run in a terminal",

		"setup.welcome":    "Welcome to tour. Two quick choices before we start.",
		"setup.lang":       "Language",
		"setup.detected":   "detected from %s",
		"setup.llm":        "LLM that writes the tours",
		"setup.found":      "found",
		"setup.notFound":   "not installed",
		"setup.noKey":      "%s is not set",
		"setup.notRunning": "not running",
		"setup.sendsCode":  "The code of the repository you tour will be sent to the selected LLM.",
		"setup.local":      "Runs on this machine. Your code is not sent anywhere.",
		"setup.none":       "No LLM is available yet. You can choose one later with `tour setup`.",
		"setup.keys":       "↑↓ Select   Enter Confirm   ← Back   q Quit",
		"setup.saved":      "Saved settings to %s",
		"setup.notYet":     "Note: generating tours is not implemented yet. The LLM you chose will be used once it is.",
	},
	JA: {
		"key.back":    "← 戻る",
		"key.next":    "→ 次へ",
		"key.last":    "最後の手順です",
		"key.open":    "o エディタで開く",
		"key.quit":    "q 終了",
		"scroll.more": "▼ 続きあり  ↑↓",
		"scroll.top":  "▲ ↑↓",
		"chapter":     "第%d章  %s",
		"question":    "問い",
		"think":       "考えてから → で次へ",

		"basis.fact":     "確かめた",
		"basis.inferred": "読み取れる",
		"basis.opinion":  "シニアの見方",

		"tag.example":        "例",
		"diff.summary":       "差分  +%d −%d",
		"schema.title":       "スキーマ",
		"schema.desc":        "%s を %s と %s で当てた結果",
		"schema.newTable":    "新しいテーブル",
		"change.added":       "追加",
		"change.removed":     "削除",
		"change.changed":     "変更あり",
		"change.same":        "変更なし",
		"schema.column":      "列",
		"schema.type":        "型",
		"schema.constraints": "制約",
		"schema.index":       "索引",
		"schema.unique":      "一意の索引",
		"rows.added":         "緑の列は今回の変更で増えた列",
		"target.branched":    "（%s から分岐）",
		"target.summary":     "%d コミット · %d ファイル変更（+%d −%d）",

		"err.fileNotFound": "ファイルが見つかりません: %s（%s）",
		"err.lineRange":    "行の指定が範囲外です: %s L%d–%d（全 %d 行）",
		"err.noDiff":       "差分が見つかりません: %s",
		"err.noHunk":       "指定したハンクがありません: %s %v",
		"err.noTable":      "テーブルがありません: %s",
		"err.unsafeSQL":    "安全のため実行しません: %s",
		"err.migration":    "マイグレーションを当てられません: %s（%v）",
		"err.schema":       "スキーマを読めません: %v",

		"editor.copy": "作業ツリーが %s と違うため、読み取り専用のコピーを開きました",
		"editor.fail": "エディタを起動できません: %v",
		"needTTY":     "tour は端末の中で実行してください",

		"setup.welcome":    "tour へようこそ。最初に 2 つだけ決めます。",
		"setup.lang":       "表示の言語",
		"setup.detected":   "端末の設定（%s）から選びました",
		"setup.llm":        "ツアーを作る LLM",
		"setup.found":      "見つかりました",
		"setup.notFound":   "入っていません",
		"setup.noKey":      "%s が未設定",
		"setup.notRunning": "起動していません",
		"setup.sendsCode":  "ツアーにするリポジトリのコードが、選んだ LLM に送られます。",
		"setup.local":      "このマシンの中で動くので、コードは外に送られません。",
		"setup.none":       "使える LLM がまだありません。あとで `tour setup` で選べます。",
		"setup.keys":       "↑↓ 選ぶ   Enter 決定   ← 戻る   q 終了",
		"setup.saved":      "設定を %s に保存しました",
		"setup.notYet":     "補足: ツアーの生成はまだ実装していません。選んだ LLM は、実装したときに使います。",
	},
}
