**変更前（base）が守っている約束**
- `findByStatusUpdatedBefore(status, before)` の呼び出し元は照合バッチだけ（`src/jobs/reconcilePendingPayments.ts:17-20`）。「決済中のまま 10 分たった注文」を探すための道具。索引 `orders_status_updated_at (status, updated_at)` も、コメントに「照合バッチのため」とある（`db/migrations/001_create_orders.sql:11-12`）。
- `updated_at` は「最後に保存した時刻」。`save` のたびに `now().toISOString()` で上書きされる（base `sqliteOrderRepository.ts:51`）。注文日でも決済日でもない。
- 状態の動き（`orderService.ts:33`）: CART / PAYMENT_FAILED → PENDING_PAYMENT → PAID / PAYMENT_FAILED。PAYMENT_FAILED はもう一度確定できる。PAID はそれ以上動かない。照合バッチが触るのは PENDING_PAYMENT だけ。
- 決済の冪等キーは必ず `paymentKeyFor` で作る（`order.ts:13-19` のコメント）。
- handler の慣習: handler が業務の例外を `instanceof` で HTTP の状態コードに変え、知らない例外は投げ直す。server がそれを 500 にする（`handlers/checkout.ts`、base `server.ts:14-20`）。
- 設定: `createApp` は環境変数を読み、どれも既定値を持つ（`app.ts:7-12`）。必須の環境変数は 1 つもない。
- 金額は円の整数。DB の CHECK で守っている（`001_create_orders.sql:4-6`）。状態も CHECK で守っている。

**変更との突き合わせ**
- 既存の約束: 照合バッチの振る舞いは変わらない。since が undefined なら条件が増えず、WHERE の並びが変わるだけで意味は同じ（head `sqliteOrderRepository.ts:54-69`、`orderRepository.ts:31-36`）。既存のテストは書き換えられておらず、足されただけ。
- 接続や設定: `adminToken: env.ADMIN_TOKEN` には既定値も起動時のチェックもない（head `app.ts:15`）。handler の比較は `req.token !== deps.adminToken`（head `handlers/orderExport.ts:26`）。ADMIN_TOKEN が未設定でヘッダーもなければ、`undefined !== undefined` が false になり、**認証を通ってしまう**。この場合を確かめるテストはない（テストは `adminToken: 'secret'` だけ）。リポジトリの中に ADMIN_TOKEN を設定する場所はない（README の説明だけ）。
- 書かれた慣習: reports は README に「読み取り専用」と書かれ、実際に find しか呼んでいない。冪等キーも `paymentKeyFor` を使っている。handler の例外の扱いは checkout と同じ型。ただし返り値の型は、JSON 前提の `HttpResult` ではなく、新しく作った `RawHttpResult` になっている。
- 型や制約: 既存のメソッドに「null なら全状態」という意味を足したため、名前（ByStatus）と中身がずれた。別のメソッド `findUpdatedBetween` にする案がある。認証は `adminToken: string | undefined` という型が「未設定」を許している。起動時に必須にすれば、型で防げる。
- 同時実行など: 読み取りだけで、SELECT は 1 回。マイグレーションもない。新旧が共存しても問題はない。気になるのは 2 つ。期間に上限がない（全件をメモリに載せる）。状態を指定しない検索は、索引の先頭の列（status）を使えない（推測）。
- 意味のずれ: 「期間内に更新された」は注文日ではない。PAYMENT_FAILED のあと翌月に決済し直した注文は、先月の CSV から消える。CART も出力される。経理の月次の突き合わせに使うには、出すたびに結果が変わる（推測）。
- 機械的な変更: app.ts の返り値に 1 項目を足した、server の import、README、InMemory 実装の追随。
