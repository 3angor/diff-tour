**変更前（base）が守っていた約束**
- `PENDING_PAYMENT` は「課金されたかどうか分からない」状態。注文確定は課金の前にこの状態を保存する（base `src/orders/orderService.ts:37-40`）。タイムアウトや障害のときもこの状態のまま残して、照合バッチに任せる（同 `:56-58`）。テストもこの振る舞いを守っている（base `test/orderService.test.ts:37-41`）。
- 照合バッチが触るのは、更新から 10 分を過ぎた `PENDING_PAYMENT` だけ（base `src/jobs/reconcilePendingPayments.ts:5-6, 17-20`）。バッチは 5 分ごとに動く（base `src/server.ts:23-25`）。だから注文は最大 15 分ほど決済中のまま残る。コミットメッセージの「最大 15 分」と合っている。
- 書かれていない約束がある。10 分を過ぎた `PENDING_PAYMENT` を書き換えるのは照合バッチだけ。注文確定は `CART` と `PAYMENT_FAILED` しか受け付けないので（base `orderService.ts:33-35`）、決済中の注文には触らない。バッチは読んでから `findCharge` の通信を待ち、そのあと条件なしで保存している（base `reconcilePendingPayments.ts:22-31`）。これで問題が起きなかったのは、ほかに書き手がいなかったから。
- 冪等キーは `paymentKeyFor` で「必ずここで作る」と書かれている（base `src/orders/order.ts:13-19`）。
- 層の分け方は README に書かれている。`handlers/` は HTTP との変換、`orders/` は業務ロジック（base `README.md:12-17`）。実際に `handleCheckout` は `OrderService` に処理を任せて、エラーを HTTP の応答に置き換えているだけ（base `src/handlers/checkout.ts:10-35`）。
- マイグレーションは `openDatabase` が起動時に番号順に当てる（base `src/db.ts:15-26`）。テストも `openDatabase(':memory:')` を通るので、同じ仕組みでテーブルができる。
- 状態の遷移は、アプリのコードの手順（読む→確かめる→保存）で守っている。データベースが守っているのは、`status` に入る値の CHECK 制約だけ（base `db/migrations/001_create_orders.sql:6`）。`save` は行全体を上書きする upsert（base `src/orders/sqliteOrderRepository.ts:39-52`）。

**変更（head）との突き合わせ**
- 既存の約束: `paymentKeyFor` の出力は変わっていない。既存のテストは 1 行も書き換えられていない（diff で確認）。逆向きの関数 `orderIdFromPaymentKey` は `paymentKeyFor` の隣に置かれ、接頭辞の定数を共有している（head `order.ts:13-28`）。「必ずここで作る」の決まりに従っている。
- 崩れた約束: 新しく Webhook という第 3 の書き手ができた。そのため「10 分を過ぎた PENDING はバッチだけが触る」という前提が崩れている。バッチが `findCharge` を待つ間に Webhook が `PAID` を保存し、そのあとバッチが古い読み取り結果で上書きする時間の窓がある（inferred）。課金ありと判定されれば同じ値で上書きするだけで済む。課金なしと判定されると、`PAID` が `PAYMENT_FAILED` に戻る。ただし、決済サービスが「成功」と通知した課金を「見つからない」と答えることがあるかどうかは、このリポジトリからは分からない。
- 同じイベントの二重処理: `has` で確かめてから処理し、最後に `record` する 2 段階になっている。`record` の戻り値（重複なら false）は使われていない（head `paymentWebhook.ts:45-47, 60`）。本当の守りは `confirmCharge` の「`PENDING_PAYMENT` のときだけ」という確認（head `:73-78`）。今これで安全なのは、node:sqlite が同期で動き、プロセスが 1 つだからと考えられる（inferred）。
- 順番: 注文を更新してから、イベントを記録している（head `:59-60`）。この順番は正しく、テストもある（head `test/paymentWebhook.test.ts:89-104`）。
- 接続と設定: 鍵は `PAYMENT_WEBHOOK_SECRET` から `app.ts` を経て渡される。鍵がなければ 503 を返す。本番では `now` を渡さないので、`deps.now?.()` が undefined になり、`verifyWebhookSignature` の既定値 `new Date()` が使われる。
- 型や制約で守れないか: 「PENDING のときだけ PAID」は条件付きの UPDATE で守れる。2 つのテーブルは同じ shop.db にあるので、イベントの記録と注文の更新を 1 つのトランザクションにまとめることもできる。
- 慣習からのずれ: `handlers/paymentWebhook.ts` が状態の遷移という業務ロジックを持っていて、README の層分けからずれている。`test/webhookSignature.test.ts` には、注文キーとイベントの記録のテストも混ざっている。既存のテストはモジュールごとにファイルを分けている。
- 運用: イベントのテーブルを消す仕組みがない。`PAYMENT_FAILED` の注文に成功の通知が届いても、`console.warn` を出すだけになる。
- 機械的な変更: `app.ts` の組み立て、README、`order.ts` の定数化、`server.ts` の振り分け。checkout の経路は変わっていない。
