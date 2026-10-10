# ツアー JSON の形式

出力は 1 つの JSON オブジェクト。

```
Tour = {
  "version": 1,                    // 形式の版。必ず 1
  "title": string,                 // ツアーの題
  "base": string,                  // 指示で与えられる base のブランチ名をそのまま書く
  "head": string,                  // 指示で与えられる head のブランチ名をそのまま書く
  "chapters": [ { "title": string, "steps": Step[] } ]
}

Step = {
  "title": string,
  "question"?: true,               // 問いだけを見せる手順。答えは必ず「次の手順」に置く
  "blocks": Block[]
}
```

Block は次のどれか 1 つ（キーで種類を表す）:

| ブロック | 意味 |
|---|---|
| `{ "p": string, "basis"?: "fact" \| "inferred" \| "opinion" }` | 段落。basis は根拠。fact=コード・テスト・コミットメッセージで確かめた / inferred=コードの構造から言える / opinion=一般的な設計の見方（このリポジトリでは確かめていない） |
| `{ "quote": string }` | 引用（コミットメッセージ、レビューコメントなど）。`\n` で段落を分ける |
| `{ "list": string[] }` | 箇条書き |
| `{ "pre": string }` | 整形済みの図（処理の流れ、時系列）。1 行は全角換算でおよそ 38 文字（半角 76 文字）以内 |
| `{ "table": { "head": string[], "rows": string[][] } }` | 表 |
| `{ "code": { "file": string, "at"?: "base" \| "head", "lines": [開始, 終了], "show"?: [開始, 終了], "note"?: string } }` | リポジトリの実際のコード。lines は強調する行、show は表示する範囲（省略時は前後 2 行）。at の既定は head。note には「変更なし」など |
| `{ "diff": { "file": string, "hunks"?: number[] } }` | そのファイルの差分。hunks は差分の中のハンク（`@@` で始まる塊）の 0 始まりの番号。省略時はすべて |
| `{ "snippet": { "label": string, "text": string, "lang"?: string } }` | リポジトリにないコード（素直な書き方の例、修正案など）。label に「このリポジトリにないコード」であることを必ず書く。lang は色分けに使う言語（例: "typescript"、"sql"。省略時は中身から推測） |
| `{ "schema": { "migrations": string, "tables"?: string[], "focus"?: string[] } }` | DB のスキーマ。ツールがマイグレーションの SQL を base と head で当てて、変化を表で描く。migrations はディレクトリ、focus は "テーブル.列" |
| `{ "rows": { "tag"?: string, "label": string, "head": string[], "data": string[][], "added"?: string[] } }` | データの例。tag の既定は「例」（説明用に作ったデータ）。added は今回増えた列 |
| `{ "target": true }` | 対象の変更の要約（コミット一覧と変更量）。ツールが git から作る。最初の手順に置く |

文字列の中では `` `コード` `` と `**強調**` が使える。

## 守ること

- `file` はリポジトリのルートからの相対パス（例: `src/app.ts`）。
- `lines` と `show` は、指定した時点（base / head）のファイルに**実在する行番号**にする。ツールが検証し、範囲外ならエラーとして表示される。行番号は推測せず、ファイルを読んで確かめる。
- `diff` の `hunks` は、渡された差分に実在するハンクの番号にする。
- 1 つの手順は 1 画面（およそ 40 行）に収まる量にする。
