# tour（試作）

git の変更を、シニアエンジニアの頭の中を追体験するツアーとして見せる TUI。
いまは手書きのツアーデータ（`tours/*.json`）を再生するだけの試作。依存パッケージなし。

```sh
node src/main.ts tours/payment-retry.json --repo ../tour-sample-shop   # 決済のリトライ
node src/main.ts tours/coupons.json --repo ../tour-sample-shop         # クーポン（DB のスキーマ変更）
```

操作: `→` 次へ / `←` 戻る / `o` エディタで開く / `q` 終了（はみ出したときだけ `↑↓` でスクロール）

- 読み取り専用。表示するコードは毎回 `git show` / `git diff` で実物から読む
- スキーマは、マイグレーションの SQL をメモリ上の SQLite に変更前と変更後で当てて、実際のテーブル定義を比べて表示する
- `o` は `$VISUAL` / `$EDITOR` で開く（`TOUR_EDITOR="code -g {file}:{line}"` のように指定もできる）。
  作業ツリーの内容がツアーの時点と違えば、その時点の内容を `~/.cache/tour/` に読み取り専用のコピーとして書き出して開く
- 幅が 110 桁以上なら、コードと説明を左右に並べる
- `--dump --cols 100 --rows 30 [--plain]` で全手順を標準出力に書き出す（確認用）
