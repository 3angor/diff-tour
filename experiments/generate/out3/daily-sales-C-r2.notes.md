**1. 最初の仮説**
- 毎朝の cron で前日の売上（件数と金額）を JSON で出す CLI を足す変更。運用の人が「昨日いくら売れたか」を DB を直接叩かずに見られるようになる。
- コミットメッセージには「Without an argument it reports the previous day in Japan time」とある（commits.txt:4）。だから「1 日」は日本時間の暦日を指すはず、という仮説を立てる。

**2. 変更前のコードが守っている約束**
- `updated_at` は `this.#now().toISOString()` で保存している。中身は UTC で末尾が `Z` の ISO 文字列になる（base `src/orders/sqliteOrderRepository.ts:51`）。`order.updatedAt` の値は保存に使わず、保存した時刻が入る。
- 列は `updated_at TEXT NOT NULL` で、`(status, updated_at)` に索引がある。照合バッチのための索引（base `db/migrations/001_create_orders.sql:8,11-12`）。
- 金額は円の整数で持ち、`CHECK >= 0` がかかっている（同 `001_create_orders.sql:4-5`）。
- PAID になったあとは状態が変わらない。checkout は CART / PAYMENT_FAILED のときしか受け付けない（base `src/orders/orderService.ts:33-35`）。照合バッチが触るのは PENDING_PAYMENT だけ（base `src/jobs/reconcilePendingPayments.ts:17-20`）。
- ただし照合バッチで PAID になった注文は、`updated_at` がバッチの実行時刻になる。支払いからの遅れは 10 分以上（base `reconcilePendingPayments.ts:6,25,31`）。
- `openDatabase` は開くたびに、まだ当てていないマイグレーションを当てる（base `src/db.ts:7-27`）。DB の場所の既定値は `DATABASE_PATH ?? 'shop.db'`（base `src/app.ts:7`）。

**3. 変更と約束の突き合わせ**
- `salesOn` は `${date}T00:00:00` から `${next}T00:00:00` までの範囲を、タイムゾーンを付けずに文字列で比べる（head `src/reports/dailySales.ts:61`）。保存している値は UTC なので、この範囲は **UTC の 1 日** になる。
- 一方、CLI は日付を日本時間で決める（head `src/reports/printDailySales.ts:8-12`）。「日」の定義が 2 か所で食い違っている。
- 結果として、日本時間 0〜9 時の売上が前日の分に入る。さらに朝 9 時より前に cron を回すと、まだ終わっていない時間帯を含んだ数字が出る（推測。コードの構造から言えるが、動かしてはいない）。
- テストはすべて 03:00Z 前後（日本時間の昼）に注文を置いているので、この食い違いを見分けられない（head `test/dailySales.test.ts:23-24,39-41`）。

**4. 8 つの分野**

| 分野 | 判定 | 根拠 |
|---|---|---|
| 1 約束 | 兆しあり（重い） | 時刻のタイムゾーンが食い違っている。「PAID は更新されない」という前提はコードで確かめた |
| 2 失敗 | 小さな兆し | `DATABASE_PATH` を間違えると、空の DB を作って 0 件 0 円を正常終了で出す可能性がある（推測。node:sqlite の既定の振る舞いは確かめられない）。日付の誤りは終了コード 2、`finally` で close している |
| 3 出したあと | 兆しあり | 誤った数字が外（日報）に出る。戻せるのはコードだけ。誰も気付かない形で起きる |
| 4 信頼の境界 | 十分 | 引数は `parseReportDate` で検証し、値はバインドしている |
| 5 規模 | 十分 | SQL で集計し、索引の範囲で絞っている |
| 6 次に変える人 | 小さな兆し | タイムゾーンの知識が CLI 側だけにある。それ以外は層の分け方が README と合っている |
| 7 テスト | 兆しあり | 境界の時刻を試していない。リポジトリの本物の `save` を通している点は良い |
| 8 AI | 兆しなし | 使っている API は Node 標準。頼まれていない抽象はない。README と package.json の追随は機械的 |

**5. ツアーで厚く語るもの**
- 1・3・7 に絞る。「日本時間の前日」と言いながら UTC で切っている問題と、それをテストが見逃した理由。
- 気にしなくてよいもの: 規模、SQL インジェクション、金額の型、日本の夏時間。
- 良い点: SQL での集計と索引の使い方、`parseReportDate` が繰り越しに気付いていること、終了コードの分け方、テストが本物のリポジトリを通していること。
