**変更前のコードが守っている約束（base）**
- `orders.updated_at` は TEXT 型。値は必ず `Date#toISOString()` が書く UTC の文字列（例 `2026-10-10T03:00:00.000Z`）になる。書き込み口は `SqliteOrderRepository.save` の 1 か所だけ（base `src/orders/sqliteOrderRepository.ts:51`）。DB の制約はなく、アプリのコードで守っている（base `db/migrations/001_create_orders.sql:8`）。
- `updated_at` と比べる値も `toISOString()` で作っている（base `sqliteOrderRepository.ts:57` `findByStatusUpdatedBefore`）。書く値と比べる値を同じ形にそろえる慣習になっている。
- `(status, updated_at)` の索引は照合バッチ用（base `001_create_orders.sql:11-12`）。
- PAID になる経路は 2 つだけ。注文確定（base `src/orders/orderService.ts:49-61`）と照合バッチ（base `src/jobs/reconcilePendingPayments.ts:25-31`）。checkout は CART と PAYMENT_FAILED しか受け付けず（`orderService.ts:33`）、照合バッチは PENDING_PAYMENT しか拾わない。そのため、今は PAID の注文がもう一度 save されることはない。これは DB の制約ではなく、コードの手順で守られている。
- 照合バッチは 10 分より古い PENDING_PAYMENT を PAID にする（`reconcilePendingPayments.ts:6`）。この経路では `updated_at` が実際に課金された時刻より 10 分以上あとになる。
- 金額は円の整数で持つ（`001_create_orders.sql:4`）。
- DB のパスは `env.DATABASE_PATH ?? 'shop.db'` から決める（base `src/app.ts:7`）。`openDatabase` は開くたびに、まだ当てていないマイグレーションを当てる（base `src/db.ts:7-27`）。
- README の構成の節に、ディレクトリの役割が書いてある（base `README.md:10-17`）。

**変更との突き合わせ（head）**
- **一番大きい問題（fact）**: CLI は日付を「日本時間の昨日」で決める（head `src/reports/printDailySales.ts:8-12`）。ところが集計の範囲 `${date}T00:00:00`〜`${nextDate}T00:00:00` は、保存されている値が UTC なので、UTC の 1 日になる（head `src/reports/dailySales.ts:61`）。日本時間で見ると 09:00〜翌 09:00 の 1 日を集計している。
- 毎朝 9 時より前に動かすと、日本時間の 06:00〜09:00 のような時間帯の注文がどの日のレポートにも入らない（inferred。定期実行の時刻はリポジトリにない。「毎朝」はコメント `printDailySales.ts:6` にある）。
- テストの注文はすべて 03:00Z（日本時間 12 時）で、境界を試していない（head `test/dailySales.test.ts:23-24,39-41,47-48`）。`yesterdayInTokyo` のテストは単独で、集計と組み合わせていない。
- 比べる値の形 `T00:00:00`（Z なし）は既存の慣習（`toISOString()`）と違う。文字列を辞書順で比べるので今は偶然うまく動いているが、壊れやすい（inferred）。
- 「PAID は以後更新されない」という前提は、今のコードでは成り立っている（fact）。ただし約束はコメントにしかなく、照合バッチを通った分は日付がずれうる（inferred）。`paid_at` 列を足し、CHECK 制約で守る案が比べる対象になる（opinion）。
- 索引: `status = ? AND updated_at` の範囲という形なので、索引を使える形にはなっている（inferred。EXPLAIN は確かめていない）。
- 実在する日付かの確認（`dailySales.ts:22-31`）、終了コード 2 の扱い、`finally` での close、DATABASE_PATH の扱いは `app.ts` と同じ（fact）。
- 同時実行と二重実行: 読むだけの SELECT 1 本なので害はない。ただし 9 時より前に動かすと、まだ終わっていない範囲を集計するので、動かす時刻によって結果が変わる（inferred）。CLI も `openDatabase` 経由でマイグレーションを当てる（fact）。
- 機械的な変更: README の 2 つのハンクと package.json の 1 行。振る舞いは変えていない（fact）。既存のテストに書き換えはない（diff に既存ファイルの test の変更なし。fact）。
