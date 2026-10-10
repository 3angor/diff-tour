# 決済リトライ変更（payment-retry）のレビュー前調査

## 結論
重大な問題が 2 つあります。

1. **タイムアウトの扱い**：課金されたか分からない注文を `PAYMENT_FAILED`（未課金）にしてしまいます。照合バッチは `PENDING_PAYMENT` しか見ないので、「お金は引かれたのに注文は失敗のまま」という状態が誰にも直されずに残ります。
2. **`head/` と `change.diff` の食い違い**：`head/` では SQLite による永続化がまるごと消えています。しかし `change.diff` にその削除は含まれていません。

---

## 0. 前提の食い違い（確認済み）
- **diff の変更前の内容が `base/` と合いません。**
  - diff の `src/app.ts` の変更前側は `InMemoryOrderRepository` を import していて、`db` を返していません。ハンクも `@@ -1,14`（14 行）です（change.diff:6, 26）。
  - 実際の `base/src/app.ts` は 16 行あり、`openDatabase` と `SqliteOrderRepository` を使って `db` を返しています（base/src/app.ts:1-3, 7-8, 15）。
- **`head/` にない `base/` のファイル**：`db/migrations/001_create_orders.sql`、`src/db.ts`、`src/orders/sqliteOrderRepository.ts`、`test/sqliteOrderRepository.test.ts`。
  - `.gitignore` から `*.db` が消えています（base/.gitignore:2）。
  - README から `db/migrations/` の行が消えています（base/README.md:17）。
- **推測**：diff は `base/` より古い時点（永続化が入る前）から取られていると思われます。そうだとすると、この 2 コミットをそのまま取り込むと次のどちらかになります。
  - コンフリクトが起きる
  - `head/` のツリーをそのまま採用した場合、永続化を外すという差し戻しが起きる
- どちらの意図なのか、作者への確認が必要です。

---

## 1. 変更前（base）が守っている約束

### 状態の意味
- **`PENDING_PAYMENT`**：「課金したかどうかがまだ分からない」状態。照合バッチの対象です（base/src/orders/orderService.ts:56-57、base/src/jobs/reconcilePendingPayments.ts:17-20）。
- **`PAYMENT_FAILED`**：「課金されていないと確定した」状態。もう一度確定しても二重課金にならないことを意味します。この状態になる経路は次の 2 つだけです。
  - カード拒否（`PaymentDeclinedError`）。何度送っても結果は同じです（base/src/payment/gateway.ts:20）。
  - 照合バッチが「課金記録なし」を確かめたとき（base/src/jobs/reconcilePendingPayments.ts:27-29 のコメント「二重課金にはならない」）。
- **`checkout`** できるのは `CART` か `PAYMENT_FAILED` のときだけです（base/src/orders/orderService.ts:33）。

### エラーの意味（gateway.ts の docコメント）
- **`PaymentUnavailableError`**：5xx・429・接続失敗。「課金はされていない」（base/src/payment/gateway.ts:31）。接続失敗は「届いていない」とされています（base/src/payment/httpGateway.ts:67）。
- **`PaymentTimeoutError`**：「リクエストは届いていて、課金が済んでいる可能性がある」（base/src/payment/gateway.ts:42）。
- **冪等キー**：同じキーで送った課金は 1 回分として扱われます。ただし保持期間は 24 時間です（base/src/payment/gateway.ts:6）。
  - キーは必ず `paymentKeyFor` で作ります。注文確定と照合バッチが同じキーを使うためです（base/src/orders/order.ts:13-18）。

### 不変条件
- **二重確定の防止**：課金の前に `PENDING_PAYMENT` を保存しておくので、連打された 2 回目は状態チェックで弾かれます（base/src/orders/orderService.ts:37-40）。
- **プロセスが落ちたとき**：`PENDING_PAYMENT` が DB に残っているので、照合バッチが拾えます（同 :38）。これは SQLite に永続化していることが前提です（base/src/app.ts:7-8）。
- **照合バッチの待ち時間**：更新から 10 分以内の `PENDING_PAYMENT` には触りません。処理中かもしれないからです（base/src/jobs/reconcilePendingPayments.ts:5-6）。5 分ごとに実行されます（base/src/server.ts:23-25）。
- **DB の制約**：
  - 金額は 0 以上の整数（base/db/migrations/001_create_orders.sql:4-5）
  - status は 4 種類のどれか（同 :6）
  - 照合用のインデックスがある（同 :11-12）

### 失敗したときの扱い
- **拒否**：`PAYMENT_FAILED` を保存して再 throw します。
- **それ以外（障害・タイムアウト・想定外の例外を含む）**：`PENDING_PAYMENT` のまま再 throw し、照合バッチに任せます（base/src/orders/orderService.ts:51-58）。
- **HTTP**：障害・タイムアウトは `503 payment_pending`「決済の結果を確認しています…注文履歴をご確認ください」を返します（base/src/handlers/checkout.ts:24-31）。「結果は未確定」という意味をそのまま利用者に伝えています。

### 既存テストが守っている振る舞い
- 「タイムアウトしたら PENDING_PAYMENT のまま残す（課金されたか分からないため）」（base/test/orderService.test.ts:37-41）
- 拒否されたら `PAYMENT_FAILED` になり、やり直すと `PAID` になる（同 :29-35）
- 決済中の注文はもう一度確定できない（同 :43-48）
- 冪等キーは `order-<id>`（同 :23-27）
- 照合：課金記録があれば `PAID`、なければ `PAYMENT_FAILED`。10 分以内は触らない（base/test/reconcilePendingPayments.test.ts）
- SQLite：保存して読み出せる。負の金額は CHECK で弾かれる（base/test/sqliteOrderRepository.test.ts）
- `handleCheckout` のテストはありません（確認済み）。

---

## 2. 変更との突き合わせ

### 破られている約束

**(A) `PAYMENT_FAILED` の意味が変わる（確認済み）**
- `catch` で、エラーの種類に関係なく `PAYMENT_FAILED` を保存するようになりました（head/src/orders/orderService.ts:50-55）。
- そのため「課金済みかもしれない」タイムアウトも「未課金と確定」として記録されます。gateway.ts:42 の定義と矛盾します。
- 照合バッチは `PENDING_PAYMENT` しか探さないので（head/src/jobs/reconcilePendingPayments.ts:17-18）、**課金済みなのに `PAYMENT_FAILED` の注文は二度と直されません**。利用者がやり直さなければ「引き落とし済みで未注文」のままです。
- 照合バッチのコメント「課金の記録がないので…二重課金にはならない」（同 :28）が前提にしていた意味も崩れます。

**(B) 二重課金の可能性（一部推測）**
- タイムアウトのあとに利用者がやり直すと、同じキー `order-<id>` で再送されます（head/src/orders/orderService.ts:46）。
  - 24 時間以内なら、決済サービス側で 1 回分にまとめられるはずです（gateway.ts:6）。
  - **24 時間を過ぎてからやり直すと、新しい課金として扱われる可能性があります**。
- 前の要求がまだ処理中のうちに同じキーで再送したときの決済サービスの振る舞い（409 など）は、コードからは分かりません。
  - 409 なら `HttpPaymentGateway` は想定外のステータスとして汎用 `Error` を投げます（head/src/payment/httpGateway.ts:35-36）。
  - その結果、`PAYMENT_FAILED` が保存され、ハンドラは汎用エラーを再 throw して 500 を返します（head/src/handlers/checkout.ts:33、head/src/server.ts:17-19）。ここは推測です。

**(C) 想定外の例外まで `PAYMENT_FAILED` になる（確認済み）**
- base では、決済以外の例外も `PENDING_PAYMENT` のまま照合に回していました。
- head では、たとえば次のような場合でも `PAYMENT_FAILED` になります。
  - 200 応答の `res.json()` が失敗した場合。この場合は課金済みです（httpGateway.ts:38）。
  - 2xx/402/5xx/429 以外の応答で汎用 `Error` が投げられた場合（httpGateway.ts:35-36）。
- コメントの前提「ゲートウェイ側でリトライ済みなので」（head/src/orders/orderService.ts:51）は、これらには当てはまりません。

**(D) 隠れた結合（確認済み）**
- `OrderService` は `PaymentGateway` インターフェースを受け取るだけです（head/src/orders/orderService.ts:24）。それなのに、正しく動くかどうかが「`RetryingPaymentGateway` で包まれていること」に依存するようになりました。
- 包まれているのは `app.ts` だけです（head/src/app.ts:8-14）。テストでは包まれていない `ScriptedPaymentGateway` を直接渡しています。
- リトライ後に失敗しても、リトライなしで失敗しても、どちらも同じ扱いになります。

**(E) HTTP 応答の意味と互換性（確認済み）**
- `503 payment_pending` が `502 payment_failed`「決済に失敗しました。…もう一度お試しください」に変わりました（head/src/handlers/checkout.ts:24-31）。
  - タイムアウトでは事実と違う案内になります。
  - `503` や `payment_pending` を見て判断しているクライアントがあれば壊れます。ハンドラのテストがないので、検出もされません。

**(F) 永続化の喪失（0 を参照。`head/` の内容としては確認済み）**
- `head/src/app.ts:7` は `InMemoryOrderRepository` です。再起動で注文が全部消えます。
- 「途中でプロセスが落ちても照合バッチが拾える」というコメントは残っていますが（head/src/orders/orderService.ts:36-37）、実態に合わなくなります。
- 金額と status の CHECK 制約、それを守るテストも消えます。
- **推測**：複数プロセスで動かすと、状態チェックによる連打防止がプロセスをまたいで効きません。

### 守られている約束（確認済み）
- **冪等キー**：リトライでも同じキーを使います（head/test/retryingGateway.test.ts:186-190）。キーが空なら課金しません（head/src/payment/retryingGateway.ts:31-33）。
- **カード拒否**：リトライしません（retryingGateway.ts:52-54、test :202-206）。拒否時の状態遷移も今までどおりです。
- **連打防止**：リトライ中は `PENDING_PAYMENT` のままなので、2 回目は 409 になります（head/src/orders/orderService.ts:32-39）。
- **照合バッチとのタイミング**：リトライ全体の最悪時間は約 10 秒 × 3 回 + 0.2 秒 + 0.4 秒 ≈ 30.6 秒です（retryingGateway.ts:24-25, 41、app.ts:12）。照合バッチの 10 分よりずっと短いので、照合バッチとは競合しません。
- **`findCharge`**：リトライせずに素通しします（retryingGateway.ts:47-49）。照合バッチの振る舞いは変わりません。

### 新しく生まれた約束・振る舞い
- **最大 3 回、full jitter 付きで待つ**：待ち時間は `random × 200 × 2^(n-1)` ms です（retryingGateway.ts:39-42）。
- **応答が遅くなる**：注文確定の HTTP 応答が最大で約 30 秒かかります。手前の LB やクライアントのタイムアウトのほうが短いと、利用者には失敗に見えたまま、裏で処理が続きます（推測）。
- **`PAYMENT_FAILED` は「やり直してよい」という意味になった**：コメント（head/src/orders/orderService.ts:51-52）と書き換えられたテストが前提にしています。

### 消された行・書き換えられたテストが守っていたもの
- **`if (e instanceof PaymentDeclinedError)` の分岐を削除**（change.diff:60-65）
  - 「拒否だけが確定した失敗で、それ以外は照合に回す」という仕組みそのものでした。
- **テスト「タイムアウトしたら PENDING_PAYMENT のまま残す」を逆向きに書き換え**（change.diff:141-148）
  - (A) を守っていた唯一のテストです。
  - 新しいテストが通るのは、偽の決済サービスがエラーのときに課金を記録しないからです（head/test/fakes.ts:17-19）。「タイムアウトしたが課金は済んでいた」という本題のケースは、この偽物では表現できません。
- **`base/test/sqliteOrderRepository.test.ts` が `head/` にない**（diff には出ていません）
  - 負の金額を拒否する不変条件が、テストからも消えています。

### 途中失敗・古いコードとの共存
- **`catch` 内の `save` が失敗した場合**：状態は `PENDING_PAYMENT` のまま残り、元のエラーは `save` のエラーに置き換わります。照合バッチが拾えるので、結果的には安全側です（head/src/orders/orderService.ts:53-55）。
- **デプロイ前から残っている `PENDING_PAYMENT`**：今までどおり照合バッチが処理します。
- **ローリングデプロイ中**：同じ失敗でも、古いプロセスは 503/`PENDING_PAYMENT`、新しいプロセスは 502/`PAYMENT_FAILED` を返します（推測）。

---

## 3. レビューで確認したい点
1. diff の変更前の内容が `base/` と合わない理由と、永続化（SQLite・マイグレーション）を外すのが意図かどうか。
2. タイムアウトや想定外の例外は `PENDING_PAYMENT` のまま照合に回し、`PAYMENT_FAILED` にするのは拒否だけ、という元の扱いに戻すか。
   - 戻さないなら、すぐに `findCharge` で照合して状態を決める案もあります。
3. HTTP 502 と文言の変更について、クライアントへの影響と、タイムアウト時の案内として正しいか。
4. 「タイムアウトしたが課金は済んでいた」ケースと、`OrderService` を `RetryingPaymentGateway` で包んだ状態の結合テストを追加するか。
5. 冪等キーの 24 時間の保持期限を過ぎてからやり直した場合と、同じキーで同時に送った場合の、決済サービスの仕様。
