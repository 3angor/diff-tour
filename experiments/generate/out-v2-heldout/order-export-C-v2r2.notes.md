**変更前（base）が守っていた約束**
- `findByStatusUpdatedBefore` の利用者は照合バッチだけ。状態を 1 つ指定し、「決済中のまま古くなった注文」を探すためにある（base `src/jobs/reconcilePendingPayments.ts:17-20`）。
- 索引 `orders_status_updated_at (status, updated_at)` も、この照合バッチのためのもの。マイグレーションのコメントにそう書いてある（base `db/migrations/001_create_orders.sql:11-12`）。
- `updated_at` は `save` するたびに `#now()` で上書きされる「最後に更新した時刻」で、ISO 文字列で持つ（base `src/orders/sqliteOrderRepository.ts:51`、`orderRepository.ts:24`）。注文確定・決済の失敗・照合バッチが、どれも `save` する（base `orderService.ts:40,54,61`、`reconcilePendingPayments.ts:31`）。
- 冪等キーは必ず `paymentKeyFor` で作る（base `src/orders/order.ts:13-19`）。金額は円の整数で持つ（`001_create_orders.sql:4`）。
- handler は既知のエラーを HTTP の状態コードに変え、知らないエラーは投げ直す。server がそれを 500 にする（base `handlers/checkout.ts:14-33`、`server.ts:17-20`）。
- `createApp` は環境変数に開発用の既定値を入れる（`PAYMENT_API_KEY ?? 'test-key'` など、base `app.ts:7-11`）。起動時に必須項目を確かめる処理はない。
- README の決まり：`handlers/` は HTTP との変換、`app.ts` は組み立て、`db/migrations/` は起動時に当てる（base `README.md:10-17`）。

**変更と約束を突き合わせた結果**
- 既存の約束：照合バッチの呼び出しは書き換えられていない。`status` が null でなく `since` がない場合、SQL の条件は並びが変わっただけで意味は同じ（head `sqliteOrderRepository.ts:54-69`）。InMemory 版も同じ（head `orderRepository.ts:31-36`）。既存のテストは 1 行も書き換えられていない（diff で確認）。→ 守られている（fact）。
- **認証の穴（最重要）**：`req.token !== deps.adminToken` の比較（head `handlers/orderExport.ts:26`）と `adminToken: env.ADMIN_TOKEN`（head `app.ts:15`、既定値なし）の組み合わせ。ADMIN_TOKEN を設定せずに起動すると、ヘッダーなしの要求で `undefined !== undefined` が false になり、CSV が返る。テストは `adminToken: 'secret'` の場合しか試していない（head `test/orderExportHandler.test.ts:9,22-26`）。
- 索引：新しい検索は `status` を指定しないので、先頭の列が status の索引を範囲検索に使えない可能性が高い（inferred）。`DatabaseSync` は同期 API なので、大きな期間を指定すると、その間は注文確定の要求も待たされる（inferred）。
- `updated_at` の意味：「期間内に更新された」は「最後の更新が期間内」という意味になる。照合バッチや再度の注文確定で月をまたいで動くので、過去の月の CSV もあとから変わる。経理の「月次の突き合わせ」（コミットメッセージ）に使えるかは要確認。
- 型で守れないか：「null なら全状態」は意味を値に埋めている。別のメソッド（`findUpdatedBetween`）に分けるほうが、照合バッチ側を誤用から守れる。トークンも `createApp` で必須にすれば、型と起動時の検査で守れる。
- 慣習：handler のエラーの扱い、`paymentKeyFor` の利用、円の整数（`total_amount_jpy`）、README の構成表の更新は、どれも従っている（fact）。`RawHttpResult` は既存の `HttpResult`（body を JSON にする前提）とは別の新しい型で、CSV のヘッダーを返すためと読める。
- 日付：`+09:00` 固定、`2026-02-30` を弾くための往復の検査、半開区間 `[from, to+1日)`。テストで境界を確かめている（head `test/orderExport.test.ts:20-50`）。`content-disposition` に入る from/to は、検証を通った値だけなのでヘッダーへの注入はない（fact）。
- 同時実行・二重実行・途中の失敗：読み取り専用の SELECT 1 本なので、二重に実行しても途中で落ちても書き込みは起きない。マイグレーションがないので、新旧のコードが共存しても DB の形は変わらない（fact）。
- 機械的な変更：`app.ts` 1 行、README、インターフェースのシグネチャの追随、server のルーティング。頭を使うべきなのは、認証の比較、リポジトリの検索条件、`updated_at` の意味の 3 つ。
