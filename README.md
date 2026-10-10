# tour（試作）

git の変更を、シニアエンジニアの頭の中を追体験するツアーとして見せる TUI。
いまは手書きのツアーデータ（`tours/*.json`）を再生するだけの試作。依存パッケージなし。

```sh
node src/main.ts tours/payment-retry.json --repo ../tour-sample-shop
```

操作: `→` 次へ / `←` 戻る / `q` 終了（はみ出したときだけ `↑↓` でスクロール）

- 読み取り専用。表示するコードは毎回 `git show` / `git diff` で実物から読む
- 幅が 110 桁以上なら、コードと説明を左右に並べる
- `--dump --cols 100 --rows 30 [--plain]` で全手順を標準出力に書き出す（確認用）
