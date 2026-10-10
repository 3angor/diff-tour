# 生成の品質の実験

エージェントの CLI（Claude Code の `claude -p`）が、読み取りの道具だけで、手書きの正解例（`tours/*.json`）に近いツアーを作れるかを測る。

## 安全の確保（読み取り専用）

題材の原本（`~/github/tour-sample-shop`）は使わず、複製を作って、そこから変更前と変更後のファイルを普通のディレクトリとして書き出した。

```
<作業ディレクトリ>/<branch>/
  base/         変更前（main）の全ファイル（.git なし）
  head/         変更後（feature/<branch>）の全ファイル（.git なし）
  change.diff   git diff main...feature/<branch>
  commits.txt   git log のメッセージ
```

`claude -p` は、この作業ディレクトリを cwd にして、次のフラグで実行した（`claude --help` で確かめたもの。Claude Code 2.1.291）。

```
claude -p \
  --restricted --strict-mcp-config \
  --tools "Read,Grep,Glob" \
  --permission-mode dontAsk --permission-prompts none \
  --no-session-persistence --output-format json "<プロンプト>"
```

| フラグ | 目的 |
|---|---|
| `--restricted` | コマンドやコードを実行する道具（Bash など）と WebFetch を外し、ユーザー・プロジェクトの設定を読まない。ファイルの道具を作業ディレクトリの中に閉じ込める |
| `--strict-mcp-config` | MCP サーバーを読み込まない |
| `--tools "Read,Grep,Glob"` | 使える道具を読み取りの 3 つだけにする |
| `--permission-mode dontAsk --permission-prompts none` | 確認が必要な操作は誰にも聞かずに拒否する |
| `--no-session-persistence` | セッションを保存しない |

本番の前に、安い模型（haiku）で閉じ込めを確かめた。

- 作業ディレクトリの中のファイル → 読めた
- 正解例（`~/github/tour/tours/coupons.json`）を絶対パスで読む → **拒否された**（`permission_denials` に記録）
- シェルのコマンド → 道具そのものがない

git を使わせない形にしたので、生成側は `git log` や `git blame` で履歴をたどれない。必要な履歴は `commits.txt` と `change.diff` で渡した。

## プロンプト

`prompts/` に置いた。

| ファイル | 中身 |
|---|---|
| `common.md` | テーマ・読者・方針・章立て・渡すもの |
| `format.md` | ツアー JSON の形式（`tour-format.ts` から書き起こした） |
| `output.md` | 最後に JSON だけを出す指示 |
| `stage1.md` | 方針 B の 1 段目（変更前のコードが守っている約束を取り出し、変更と突き合わせる） |

- **方針 A**: `common.md` + `format.md` + `output.md` を 1 回で渡す
- **方針 B**: まず `stage1.md` で下調べを書かせ、その結果を `common.md` に挟んで 2 回目でツアーを書かせる

実行は `run.sh <payment-retry|coupons> <A|B> <作業ディレクトリの親>`。

## 検証

`validate.mjs <tour.json> <題材の複製>` で、JSON の形、`code` の指すファイルと行の実在、`diff` のハンク番号の実在、問いの手順の位置、図の幅を確かめる。手書きの正解例 2 つは、どちらもエラー 0 で通る。

作業ディレクトリは `prepare.sh <題材> <作業ディレクトリの親> payment-retry coupons` で作る。

> **今回の実行での準備の誤り**: 実際の 4 回の実行では、`base/` に main の**先端**を置いてしまった。payment-retry は main から分かれたあとに main へ SQLite の変更が入っているので、`base/` と `change.diff`（分岐点との差分）が食い違った。coupons は分岐点＝ main の先端なので影響はない。`prepare.sh` は分岐点（merge-base）を使うように直してある。

## 再生

生成したツアーは `out/<branch>-<A|B>.json`。Go 版の形式（`"version": 1`）に合わせてある（実行時の出力には version がなかったので、あとから先頭に足した。プロンプトの `format.md` は直してある）。

```sh
cd ~/github/tour
mise exec -- go run ./cmd/tour experiments/generate/out/coupons-B.json --repo <題材の複製> --lang ja
```

複製では、ツアーの `base` / `head` と同じ名前のローカルのブランチ（`main`、`feature/payment-retry`、`feature/coupons`）が要る。4 本とも `--dump` で全手順を書き出せて、⚠（行やハンクの指定の誤り）は 0 件だった。1 画面からはみ出す手順は payment-retry B と coupons A に 1 つずつ（3 行）。

## 結果

4 回とも Claude Opus 5.5（`claude-opus-5-5`）。費用は CLI の JSON 出力にある `total_cost_usd`（API の価格での換算。実際の請求はプランによるので確かめていない）。

| | payment-retry A | payment-retry B | coupons A | coupons B |
|---|---|---|---|---|
| 手順数 / 問い | 18 / 2 | 20 / 2 | 18 / 2 | 21 / 3 |
| 検証エラー（形・行・ハンク） | 0 | 0 | 0 | 0（※） |
| 変更していないファイルを見せた数 | 4 | 6 | 3 | 4 |
| 時間 | 2.0 分 | 4.2 分（2 回） | 2.3 分 | 4.0 分（2 回） |
| 費用 | $0.46 | $1.09 | $0.55 | $1.08 |

※ coupons B は出力の JSON に文法の誤りが 1 か所あった（表の行の閉じ方 `","],["`）。手で 1 文字消して直してから検証した。

### 正解例の論点をどこまで拾えたか

payment-retry:

| 正解例の論点 | A | B |
|---|---|---|
| **仕込んだ問題**: タイムアウトでも `PAYMENT_FAILED` にして照合バッチの対象から外れる | ✔ | ✔ |
| **仕込んだ問題**: 理由付きの既存テストが逆向きに書き換えられた | ✔ | ✔ |
| 冪等キー（変更していない `order.ts` / `gateway.ts`）が安全の根拠 | ✔ | ✔ |
| 課金の前に `PENDING_PAYMENT` を保存する理由 | ✘ | ✔ |
| リトライを外にかぶせる設計（Decorator） | ✔ | ✔ |
| 待ち時間のばらつき（jitter） | ✔ | ✔ |
| 待ち時間の合計（約 30 秒） | ✔ | ✔ |

coupons:

| 正解例の論点 | A | B |
|---|---|---|
| **仕込んだ問題**: 同じ注文への二重適用・付け替えで `used_count` がずれる | ✔ | ✔ |
| 上限の確認と加算を 1 つの UPDATE で行う理由（同時実行） | ✔ | ✔ |
| `discount_amount` を `NULL` ではなく `NOT NULL DEFAULT 0` にする理由 | △（DEFAULT には触れた） | △ |
| `REFERENCES` は `db.ts` の `PRAGMA foreign_keys = ON` があって初めて効く | ✘ | ✘ |
| 金額は整数で持つという既存の慣習（`001_create_orders.sql`） | ✘ | ✘ |
| 機械的な変更（テストへの `discountAmount: 0`）の仕分け | ✔ | ✘ |
| 入れ替え中に古いコードが満額を請求する | ✘ | ✔ |
| 使用を記録の行として持つ設計（制約で守る）という選択肢 | ✘ | ✘ |
| 回数の加算と保存がトランザクションになっていない | ✔ | ✔ |

### 正解例になかったが、もっともな指摘

コードを読んで、前提が合っていることを確かめた（決済サービスの実際の振る舞いは確かめられないので、生成側も inferred / opinion にしている）。

- **coupons A・B**: `PAYMENT_FAILED` の注文にもクーポンを付けられるので、拒否された注文を割引付きで確定し直すと、**同じ冪等キーで違う金額**が決済サービスに送られる。テストの偽物は冪等キーをまとめないので気付けない。正解例より鋭い指摘
- **coupons A・B**: 全額割引だと 0 円の課金を送る。決済サービスが 4xx を返すと汎用の Error になり、注文が止まる
- **coupons B**: `apply` も注文全体を上書きするようになり、書き手が 2 つになった。複数のインスタンスでは、確定中の `PENDING_PAYMENT` が `CART` に戻りうる（1 プロセスなら割り込まれない、という条件付きの説明も正しい）
- **payment-retry B**: リトライ対象でない汎用の Error まで `PAYMENT_FAILED` になる。`OrderService` の正しさが「リトライで包まれていること」に依存するようになった
- **payment-retry B**: HTTP の応答が `503 payment_pending` から `502 payment_failed` に変わり、クライアントとの約束が変わった

### 誤り

- **payment-retry B**「差分に出ていない削除：永続化が消えている」: 上の準備の誤りによる。生成側は `base/` と `change.diff` の食い違いを正しく見つけて、「作者に確認するしかない」と書いており、入力に対しては筋の通った反応だった

### 根拠のラベル

- 全体として正直。外部の決済サービスの振る舞いは、どれも inferred か opinion にしていて、「コードからは分からない」と書いている
- 小さな誤用: 問いかけの文に `fact` を付けている（payment-retry B の問い）。ラベルを付けない段落が coupons B では 3 つ、payment-retry A では 4 つ

## 気付いたこと

1. **仕込んだ問題は 4 回とも見つけた**。変更していないファイル（照合バッチ、冪等キー、テストの偽物）も自分から読みに行った。品質は「正解例に近い」を超えて、正解例にない妥当な指摘を複数出した
2. **A と B の差は小さい**。B は「変更前の約束」の論点（`PENDING_PAYMENT` を先に保存する理由、入れ替え中の共存、汎用 Error）を多く拾ったが、費用と時間は約 2 倍。各 1 回ずつなので、差がたまたまかどうかは**確かめていない**
3. **両方とも外したもの**は「変更していない設定・慣習」の層: `db.ts` の外部キーの設定、マイグレーションのコメントにある慣習、制約で守る別の設計。問題を探すことに引っ張られて、「この変更が既存の決まりに従っているか」「スキーマで守れないか」という観点が抜ける
4. **入力の作り方に敏感**。base と差分の時点がずれると、生成側はそれを問題として報告する（今回の誤り）
5. 出力の JSON が長い（約 9〜15k 字）ため、文法の誤りが 1/4 で出た

## 再生側への指摘（未修正）

Go 版の再生ツールは、`code` の `"at": "base"` とスキーマの変更前を、`tour.base`（`main` の**先端**）から読んでいる（`internal/tour/tour.go:134`、`internal/render/blocks.go:336`）。一方、差分は `main...head`（**分岐点**との比較）で読む。payment-retry のように分岐のあとで main が進んでいると、「変更前のコード」と差分が食い違う。今回の準備の誤りと同じ種類の問題なので、再生側でも分岐点を使うべき。

## 次に直すべき点

1. 出力の検証で JSON の文法エラーや検証エラーが出たら、エラーを添えて直させる（再試行の輪を作る）。可能なら構造化出力を使う
2. プロンプトに観点の一覧を足す: 「接続や設定で決まる振る舞い（外部キーの有効化など）」「コメントに書かれた既存の慣習に従っているか」「アプリのコードではなく制約で守れないか」「機械的な変更の仕分け」
3. 方針は、B の 1 段目の内容（変更前の約束の書き出し）を A の 1 回の中で先にやらせる形も試す。2 回呼ぶより安く、B の利点を取れるかを見る
4. 根拠のラベルの規則に「問いかけの文には付けない」「すべての断定文に付ける」を足す
5. 同じ条件で各 3 回ずつ回して、ばらつきを見る（今回は各 1 回）
