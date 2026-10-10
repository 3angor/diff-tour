# 下調べ：決済の一時障害をリトライする変更（08ad4ab, cdae2ad）

**要点：** リトライの仕組み（`RetryingPaymentGateway`）そのものは、冪等キーの約束の範囲に収まっています。問題は同じ変更に含まれる `OrderService` の catch の書き換えです。「課金されたか分からない」タイムアウトを `PAYMENT_FAILED` にしたため、照合バッチという安全網から外れます。その結果、「お金は取れたのに注文は失敗」と「24 時間後の再確定で二重課金」の 2 つが起こりえます。

---

## 1. 変更前（base）が守っている約束

### 状態の意味
- **`PENDING_PAYMENT` は「課金されたかどうか、まだ分からない」という意味。** 課金の前に保存し、障害やタイムアウトのときはこの状態のまま残して、照合バッチに判断を任せる。（確認済み：base/src/orders/orderService.ts:37-40, 56-58）
- **`PAYMENT_FAILED` は「課金の記録がない」と分かっている状態。** だから、ユーザーがもう一度確定しても二重課金にならない、と照合バッチのコメントに書かれている。（確認済み：base/src/jobs/reconcilePendingPayments.ts:27-29）
  - `PAYMENT_FAILED` になる道は 2 つだけ：
    - カードの拒否（base/src/orders/orderService.ts:52-54）
    - 照合バッチが課金の記録なしと判断したとき
- **もう一度確定できるのは `CART` と `PAYMENT_FAILED` だけ。**（確認済み：base/src/orders/orderService.ts:33）
  - つまり「もう一度確定できる」ことと「課金されていないと分かっている」ことが一致している。これがこのコードの中心の不変条件。

### エラーの意味（base/src/payment/gateway.ts）
- `PaymentDeclinedError`：何度送っても結果は変わらない（:20）
- `PaymentUnavailableError`：5xx・429・接続失敗。**課金はされていない**（:31）
- `PaymentTimeoutError`：**リクエストは届いていて、課金が済んでいる可能性がある**（:42）
- 冪等キー：同じキーの課金は 1 回分として扱われる。**保持期間は 24 時間**（:6）
- `httpGateway` は、想定外の 4xx（402 以外）を素の `Error` で投げる（base/src/payment/httpGateway.ts:35-36）
  - base ではこの場合 `PENDING_PAYMENT` のまま残り、ハンドラから 500 になる（base/src/handlers/checkout.ts:33、base/src/server.ts:17-19）

### 冪等キー
- 注文確定と照合バッチが同じキーを使うため、キーは必ず `paymentKeyFor` で作る決まり。（確認済み：base/src/orders/order.ts:13-19）
- 注文 ID だけから作るので、**同じ注文は何度確定し直しても同じキー**になる。

### 照合バッチ
- 5 分ごとに実行する。（base/src/server.ts:23-25）
- 対象は「更新から 10 分以上たった `PENDING_PAYMENT`」**だけ**。（base/src/jobs/reconcilePendingPayments.ts:6, 17-20）
- 1 件の `findCharge` が失敗すると、残りの注文も処理されない。（base/src/jobs/reconcilePendingPayments.ts:22-23。既存の挙動で、今回の変更とは関係ない）

### 連打（二重実行）への備え
- 先に `PENDING_PAYMENT` を保存し、状態チェックで 2 回目を弾く。
- これは**アプリのコードの手順で守っている**。DB の制約や比較交換（CAS）はない。（推測：`get` と `save` の間に `await` があるので、本当に同時に来たリクエストは 2 つとも通りうる。base/src/orders/orderRepository.ts:17-25）
- その場合の最後の防壁は、決済サービス側の冪等キー。

### HTTP の約束
- 決済の障害やタイムアウトでは、**503 `payment_pending`**「決済の結果を確認しています…注文履歴をご確認ください」を返す。
- 「結果は未確定」とユーザーに正直に伝えている。（base/src/handlers/checkout.ts:24-31）

### 既存のテストが守っている振る舞い
- タイムアウトしたら `PENDING_PAYMENT` のまま残す（base/test/orderService.test.ts:37-41）
- カードが拒否されたら `PAYMENT_FAILED` になり、やり直せる（:29-35）
- 決済中の注文は確定できない（:43-48）
- 照合バッチ：課金の記録があれば `PAID`、なければ `PAYMENT_FAILED`。更新から 10 分たっていない注文は触らない（base/test/reconcilePendingPayments.test.ts）
- 注意：テスト用の偽ゲートウェイ（`ScriptedPaymentGateway`）は、エラーを投げるときに課金を記録しない。また、冪等な再送（同じキーなら前の結果を返す）も再現しない。（base/test/fakes.ts:15-20）
  - そのため「タイムアウトしたが実は課金済み」という状況を、そもそもテストで表せない。

---

## 2. 変更（head）との突き合わせ

### 頭を使うべき変更
1. `OrderService.checkout` の catch（head/src/orders/orderService.ts:50-55）
2. ハンドラが返すステータスと文言（head/src/handlers/checkout.ts:24-31）
3. 書き換えられたタイムアウトのテスト（head/test/orderService.test.ts:37-43）
4. `RetryingPaymentGateway` の作り（head/src/payment/retryingGateway.ts）

### 機械的な変更
- `app.ts` でゲートウェイを包むだけの組み立て（head/src/app.ts:12-18）
- 使わなくなった `PaymentDeclinedError` の import の削除
- `retryingGateway` のテストの追加

### 破られている約束

**「`PAYMENT_FAILED` は課金されていない」が破られる（重大、確認済み）**
- head/src/orders/orderService.ts:50-55 は、**どんな例外でも** `PAYMENT_FAILED` にする。
- その中には、gateway.ts:42 が「課金済みかもしれない」と明記している `PaymentTimeoutError` も含まれる。
- 一方で照合バッチは `PENDING_PAYMENT` しか見ない（head/src/jobs/reconcilePendingPayments.ts:17-18、変更なし）。だから、**タイムアウト後に実は課金されていた注文は、二度と照合されない。**
- 照合バッチのコメント「ユーザーがもう一度注文を確定しても二重課金にはならない」（:28）の前提が、もう成り立たない。

**この結果、次のことが起こりうる（決済サービスの挙動は推測を含む）**
- **ユーザーがやり直さない場合：** 課金済みなのに注文は `PAYMENT_FAILED` のまま残る。お金は取ったが注文は失敗、という状態。
- **24 時間以内にやり直した場合：** 同じ冪等キー（order.ts:17-19）なので、決済サービスが元の課金を返し、`PAID` になる。（gateway.ts:6 の約束どおりなら助かる。決済サービスが元の結果をそのまま返すかどうかは推測）
- **24 時間を過ぎてからやり直した場合：** キーの保持期限が切れているので、**二重課金**になる。

**例外の範囲が広がった（確認済み）**
- 決済の障害と無関係な例外も `PAYMENT_FAILED` になる。例：
  - httpGateway.ts:36 の想定外ステータス
  - レスポンスの JSON の解析失敗
- base では 500 を返して `PENDING_PAYMENT` に残し、照合に任せていた。
- （推測）タイムアウトの直後に同じキーで再送すると、決済サービスが「同じキーで処理中」という競合（409 など）を返すことがある。これは素の `Error` になるのでリトライされず、そのまま `PAYMENT_FAILED` になる。

**ハンドラの文言が実態と合わない（確認済み）**
- head/src/handlers/checkout.ts:24-31 は、タイムアウトでも 502 `payment_failed`「決済に失敗しました…もう一度お試しください」を返す。課金済みかもしれないのに失敗と言い切っている。
- `payment_pending` や 503 を見て分岐しているクライアントがあれば、壊れる。

**書き換えられたテストが守っていたもの（確認済み）**
- 消えた `PENDING_PAYMENT のまま残す（課金されたか分からないため）` は、上の「未確定は照合に任せる」という不変条件そのものを守っていた。
- 新しいテスト（head/test/orderService.test.ts:37-43）は、偽ゲートウェイが課金を記録せず冪等な再送も再現しないので、二重課金の危険を検出できない。
- また、このテストはリトライのラッパーを通していない。
  - `OrderService` が受け取るのは `PaymentGateway` というインターフェースなので、「ゲートウェイ側でリトライ済み」というコメント（:51）の前提は、型では何も保証されていない。

### 守られている約束
- **冪等キー：** リトライでも同じリクエストをそのまま送るので、キーは変わらない（retryingGateway.ts:115、テストは head/test/retryingGateway.test.ts:29-33）。キーの作り方もそのまま。
- **カードの拒否：** リトライしないので、gateway.ts:20 の約束と一致する（retryingGateway.ts:117, 130-132）。結果は従来どおり `PAYMENT_FAILED`。
- **連打への備え：** 先に `PENDING_PAYMENT` を保存する手順は残っている（head/src/orders/orderService.ts:36-39）。
- **照合バッチ：** `app.payments` が包んだゲートウェイになるが、`findCharge` はそのまま中身に渡すだけなので、挙動は変わらない（retryingGateway.ts:125-127、server.ts:24）。
- **照合の 10 分との関係：** 確定 1 回の最大時間は、10 秒 × 3 回 + 待ち 0.6 秒未満 ≒ 31 秒で、照合バッチの 10 分より十分短い。照合バッチと確定処理がぶつかることはない（推測を含む計算）。

### 新しく生まれた約束
- 課金のリトライは最大 3 回。待ち時間は full jitter（0〜200ms、0〜400ms）。冪等キーが空なら課金しない（retryingGateway.ts:102-121、109-111）。
  - 冪等キーが空かどうかは**実行時のチェック**で守っている。型（ブランド型など）で守ることもできる。
- 注文確定のレスポンスは最大で約 31 秒かかりうる。
  - サーバーにリクエストのタイムアウトはない（server.ts）。
  - （推測）手前のロードバランサーやクライアントが先に打ち切ると、ユーザーが押し直したときに `PENDING_PAYMENT` で 409 を受け取る。

### 同時実行・途中での失敗・古いコードとの共存
- **途中での失敗：** catch の中の `save` が失敗すると、元のエラーは失われ、状態は `PENDING_PAYMENT` に残る。照合バッチが拾うので、むしろ安全側に倒れる。
- **古いデータとの共存：** デプロイ前からある `PENDING_PAYMENT` は、従来どおり照合される。
  - ただしデプロイ後は、照合バッチが拾うのは「プロセスが落ちた」「成功後の `save` が失敗した」場合だけになり、安全網として使われる場面が狭まる。
- **同時に 2 回押された場合：** get → save が比較交換になっていない問題は base からあるもの。今回の変更でも変わらない。

### 慣習に沿っているか
- **沿っている点：** 新しいファイルは `#private` フィールドを使い、`Options` 型で設定を渡し、オプションを省略すると既定値になる。httpGateway.ts と同じ書き方で、テストのために時間（`sleep`）を注入できる点も既存のやり方に合っている。
- **沿っていない点：** gateway.ts がエラーごとに書いている「課金されたか」という意味を、orderService の新しいコメントが無視している。照合バッチのコメントや README の構成説明は、更新されていない。

### 型や制約で守る案
1. **「結果が分からない」を型で区別する。** 例えば `PaymentOutcomeUnknownError` を作る、または `PaymentTimeoutError` に印をつける。
   - そのうえで `OrderService` は、「課金されていないと確定したエラー」（拒否、`Unavailable`）のときだけ `PAYMENT_FAILED` にする。タイムアウトは `PENDING_PAYMENT` に残す。
   - これが元の約束を守る最小の修正。
2. **catch の中で `findCharge` を 1 回呼び、その場で確定させる。** これでコミットメッセージの「その場でやり直せる」が実現する。それでも分からなければ `PENDING_PAYMENT` に残す。
3. **偽ゲートウェイを直す。** `ScriptedPaymentGateway` に「タイムアウトだが課金済み」と「同じキーなら前の結果を返す」を表せるようにして、二重課金をテストで検出できるようにする。
