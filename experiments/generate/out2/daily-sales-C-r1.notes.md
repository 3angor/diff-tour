下調べが終わったので、ツアーを書く。


- **変更前の約束（base）**
  - `updated_at` は `SqliteOrderRepository.save` が `this.#now().toISOString()` で書く。値は UTC の ISO 文字列（例 `2026-10-10T03:00:00.000Z`）で、渡された `order.updatedAt` は使わない（base `src/orders/sqliteOrderRepository.ts:51`）。
  - 状態 `PAID` は 2 か所でしか作られない。注文確定（base `src/orders/orderService.ts:49,61`）と照合バッチ（base `src/jobs/reconcilePendingPayments.ts:25,31`）。PAID の注文をもう一度確定しようとしても、状態チェックで弾かれる（`orderService.ts:33-35`）。照合バッチは PENDING_PAYMENT しか拾わない（`reconcilePendingPayments.ts:17-20`）。この「PAID になったら以後更新されない」という約束はアプリのコードの手順で守っている。DB の制約はない。
  - 金額は円の整数で持ち、負の値は CHECK で弾く。状態も CHECK で 4 つの値に限っている（base `db/migrations/001_create_orders.sql:4-6`）。索引 `(status, updated_at)` は照合バッチのためのもの（同 11-12）。
  - `openDatabase` は開くたびにまだ当てていないマイグレーションを当てる（base `src/db.ts:7-27`）。`createApp` は `DATABASE_PATH ?? 'shop.db'` を使う（base `src/app.ts:7`）。
  - README に置き場所の決まり（`src/jobs/` はバッチなど）と、Node 23.6 以降という前提が書いてある（base `README.md:3,10-17`）。
- **変更との突き合わせ**
  - 既存の約束: 既存のファイルとテストは README と package.json しか変わっていない。消された行はない。
  - **時刻の帯がずれている**: `salesOn` は `${date}T00:00:00` から翌日 0 時までで絞る（head `src/reports/dailySales.ts:61`）。保存値は UTC なので、これは UTC の暦日になる。一方 CLI は日本時間の「昨日」を渡す（head `src/reports/printDailySales.ts:8-12,18`）。このため、日本時間 0:00〜8:59 の売上は前日分に数えられる。テストは 03:00Z や 05:30Z など、どちらの暦日でも同じ日になる時刻しか使っていないので、このずれを捕まえられない（head `test/dailySales.test.ts:23-24` など）。
  - 文字列の比較そのものは成り立つ。`...T00:00:00.000Z` は境界の `...T00:00:00` より大きい。
  - `updated_at` を「支払い確定日時」として扱う根拠は、上の「PAID は以後更新されない」という約束だけ。照合バッチで PAID になった注文は、課金から 10 分以上あとの時刻が入る。型や制約で守るなら、`paid_at` 列を足す設計も考えられる。
  - 接続と設定: CLI も `openDatabase` を使うので、レポートを実行するだけでマイグレーションが当たる。`DATABASE_PATH` を間違えると新しい空の DB ができて、0 件・0 円が正常終了で出ると推測する（`DatabaseSync` が存在しないファイルを作る点は外部の振る舞い）。`import.meta.main` が対応する Node の版は README の「23.6 以降」と合っているか確かめていない。
  - 慣習: 型 `DailySales.totalAmount` に「円」のコメントがあり、`order.ts` に合わせている。エラーは専用クラスにして `name` を設定していて、`orderService.ts` に合わせている。
  - 同時実行と二重実行: 読むだけの単一の SELECT なので、2 回走っても安全。ただ、朝に前日分を出したあとで照合バッチが前日の課金を PAID にすると、その注文は当日分に入る（日をまたいで移る）。
  - 機械的な変更: README に 2 行、package.json に 1 行。中身は `src/reports/` の 2 ファイルとテスト 2 ファイル。
