# クーポン変更（cd1b206, 4396aab）のレビュー前の下調べ

**一番の懸念:** 課金の冪等キーは注文 ID だけで作られている（`order-<id>`）。base では、同じキーで送る金額は変わらなかった。今回の変更で、決済に失敗した注文（`PAYMENT_FAILED`）にもクーポンを付けられるようになった。そのため、同じキーのまま違う金額で課金し直すことがある。決済サービスがこの場合にどう振る舞うかはコードからは分からず、確認が必要。

ほかの大きな論点:
- 1 つの注文にクーポンを何度でも付け直せて、そのたびに使用回数が減る。
- 割引で請求額が 0 円になることがある。
- 新旧のコードが同時に動く間、古いコードは割引を無視して満額を課金する。

コードを読んで確かめたことには【確認】、推測には【推測】を付ける。

---

## 1. 変更前（base）のコードが守っている約束

### 状態の意味
- **`CART`:** 未確定の注文。確定（checkout）できる。【確認】`base/src/orders/orderService.ts:33`
- **`PENDING_PAYMENT`:** 「課金されたかどうか、まだ分からない」状態。【確認】
  - 課金を頼む前に必ず保存する（`orderService.ts:37-40`）。
  - 障害やタイムアウトのときはこの状態のまま残し、照合バッチに判断を任せる（`orderService.ts:56-57`）。
  - テストでも守られている（`base/test/orderService.test.ts:37-41`）。
- **`PAYMENT_FAILED`:** 「課金されていないので、もう一度確定してよい」状態。【確認】
  - カード拒否のとき（`orderService.ts:52-54`）と、照合バッチが課金の記録を見つけられなかったとき（`base/src/jobs/reconcilePendingPayments.ts:27-29`）にこの状態になる。
  - reconcile 28 行目のコメントは「もう一度確定しても二重課金にならない」と書いている。これは「同じキーで同じ課金をやり直す」ことを前提にしている。
- **`PAID`:** `chargeId` を持つ、課金が済んだ状態。【確認】`orderService.ts:49-50`、`reconcilePendingPayments.ts:25-26`

### 不変条件
- **冪等キーの作り方:** キーは `paymentKeyFor(order)`（`order-<id>`）でしか作らない。注文確定と照合バッチが同じキーで問い合わせるため。【確認】
  - 定義: `base/src/orders/order.ts:13-19`
  - テスト: `base/test/orderService.test.ts:23-27`
- **決済サービス側の扱い:** 同じキーの課金は 24 時間「1 回分」として扱われる。【確認】`base/src/payment/gateway.ts:6`
- **キーと金額の関係:** base には `totalAmount` を書き換えるコードがない。そのため「1 つのキーで送る金額はいつも同じ」が暗黙に成り立っている。【確認】`totalAmount` を代入しているのは save と行の読み出しだけ。
- **二重確定の防止:** 連打しても 2 回目は状態チェックで弾く。【確認】`orderService.ts:37-38`、`base/test/orderService.test.ts:43-48`
  - これは「CART から PENDING へは checkout だけが書き換える」という前提に頼っている。【推測】
  - `get → save` は条件付き更新（CAS）ではない（`base/src/orders/sqliteOrderRepository.ts:39-52` の upsert は全列を上書きする）。
- **金額の型と範囲:** 金額は円の整数で、マイナスにならない。【確認】
  - DB の CHECK 制約: `base/db/migrations/001_create_orders.sql:4-5`
  - テスト: `base/test/sqliteOrderRepository.test.ts:14-20`
- **照合バッチの待ち時間:** 更新から 10 分以内の `PENDING_PAYMENT` には触らない。処理中かもしれないため。【確認】`reconcilePendingPayments.ts:5-6`、テスト `base/test/reconcilePendingPayments.test.ts:24-29`

### 失敗したときの扱い
| 例外 | 意味 | 注文の状態 | HTTP |
|---|---|---|---|
| `PaymentDeclinedError` | カード拒否（`gateway.ts:20`） | `PAYMENT_FAILED` | 402 |
| `PaymentUnavailableError` | 課金されていない（`gateway.ts:31`） | `PENDING_PAYMENT` | 503 |
| `PaymentTimeoutError` | 課金済みの可能性がある（`gateway.ts:42`） | `PENDING_PAYMENT` | 503 |
| それ以外の 4xx（`httpGateway.ts:35-36` の汎用 Error） | 想定外の応答 | `PENDING_PAYMENT` | 500（`server.ts:17-19`） |

HTTP への変換は `base/src/handlers/checkout.ts:15-33`。【確認】

### 慣習
- **リポジトリ:** interface（`OrderRepository`）と InMemory 実装の組で作り、テストは InMemory で書く。【確認】`base/src/orders/orderRepository.ts:3-32`
- **InMemory の返り値:** save しないで書き換えても保存内容が変わらないよう、コピーを返す。【確認】`orderRepository.ts:19`
- **マイグレーション:** 起動時に番号順に、1 本ずつトランザクションで当てる。【確認】`base/src/db.ts:15-26`。外部キー制約は有効（`db.ts:10`）。
- **金額のコメント:** 金額は「円」とコメントに明記する。【確認】`order.ts:6`、`gateway.ts:2`

---

## 2. 変更（head）との突き合わせ

### 守られている約束
- **【確認】確定処理の流れは同じ。** 「課金の前に PENDING を保存する」「失敗の種類で状態を分ける」はそのまま。変わったのは課金額だけ。`head/src/orders/orderService.ts:37-59`
- **【確認】冪等キーと照合バッチは変わっていない。** reconcile も handlers/checkout も diff に入っていない。
- **【確認】既存テストの書き換えは `discountAmount: 0` を足しただけ。** 期待値は変わっておらず、守っていた振る舞いは残っている（diff 365-432）。
- **【確認】使用回数の上限は同時に使われても超えない。** 上限の確認と加算を 1 つの UPDATE で行っている。`head/src/coupons/sqliteCouponRepository.ts:148-157`
- **【確認】既存の注文も読み書きできる。** `discount_amount NOT NULL DEFAULT 0` なので、既存の行は 0 になる。`head/db/migrations/002_add_coupons.sql:16`
- **【確認】外部キーを付けた列の追加は通る。** SQLite は、REFERENCES 付きの列を既定値 NULL で ADD COLUMN することを許している。`002_add_coupons.sql:15`

### 破られている、または弱まった約束

1. **同じ冪等キーで違う金額を送ることがある**【確認＋推測】
   - クーポンは `PAYMENT_FAILED` の注文にも付けられる（`head/src/coupons/couponService.ts:21`）。
   - 起きる流れ: 拒否、または照合バッチが `PAYMENT_FAILED` にする → クーポンを付ける → 同じ `order-<id>` で 2500 円を課金する。
   - 24 時間以内なら、決済サービスがこれを「同じ課金」と見なすおそれがある。
   - 【推測】特に危ない例: タイムアウトした課金が、照合バッチの問い合わせの後に遅れて成立していたとする。この場合、割引後の再課金は元の 3000 円の課金として返る。注文は割引 500 円・`PAID` と記録されるのに、実際には満額が取られる。
   - キーが同じで中身が違うとき、決済サービスが何を返すか（前回の結果か、エラーか）は `gateway.ts` に書かれていない。要確認。
   - テストもない。`ScriptedPaymentGateway` はキーによる重複排除を再現していない（`test/fakes.ts:15-21`）。

2. **請求額が 0 円になることがある**【確認＋推測】
   - 割引は `Math.min(割引, 注文金額)` で計算するので、請求額 0 円を許している（`couponService.ts:32`）。テストもその前提で書かれている（`head/test/couponService.test.ts:42-46`）。
   - それでも `amount: 0` のまま決済サービスに送る（`orderService.ts:44`）。
   - 【推測】多くの決済サービスは 0 円の課金を 4xx で断る。その場合は汎用 Error になり、500 を返して `PENDING_PAYMENT` で止まる。10 分後に照合バッチが `PAYMENT_FAILED` にするが、もう一度確定しても同じことが繰り返される。

3. **割引額を DB で守っていない**【確認】
   - `discount_amount` に CHECK 制約がない（`002_add_coupons.sql:16`）。`total_amount` と同じ「マイナスにしない」約束が DB では守られない。
   - 「割引は注文金額以下」もクーポン適用の時点でしか確かめていない。
   - 【推測】今は `totalAmount` を変えるコードがないが、カートを編集できるようになると、請求額がマイナスになりうる。

4. **書き込む処理が 2 つになった**【推測】
   - `apply` も `get → save`（全列の upsert）をする（`couponService.ts:19-33`）。
   - 同じプロセス内では、`node:sqlite` が同期処理なので、`apply` の get から save までは途中で他の処理が割り込まない（await しているのは解決済みの Promise だけ）。単一プロセスなら実害はほぼない。
   - 複数のプロセスやインスタンスで動かすと、次のようなことが起こりうる:
     - `apply` が古い `CART` を読む → 確定処理が `PENDING_PAYMENT` を保存する → `apply` が `CART` と割引で上書きする。
     - その結果、照合バッチが拾わなくなる。もう一度確定できてしまい、上の 1 番の問題（同じキーで違う金額）につながる。
     - あるいは、確定処理の最後の save が割引を消す。
   - base の連打対策と同じく CAS がない、という弱さを引き継いでいる。

5. **使用回数とクーポン適用が一緒に確定しない**【確認】
   - `consumeUse` の後の `orders.save` はトランザクションにまとまっていない（`couponService.ts:28-33`）。save が失敗すると、使用回数だけが減る。
   - **何度でも適用できる:** 1 つの注文に何度でも適用でき、そのたびに使用回数が増える。別のクーポンに付け替えても、前のクーポンの使用回数は戻らない。`order.couponCode` が付いているかを確かめていない（`couponService.ts:18-33`）。
   - **使わなかった分も戻らない:** 確定しなかった注文や `PAYMENT_FAILED` のままの注文の分も戻らない。`used_count` は実際には「適用された回数」を数えている。
   - 顧客ごとの使用制限もない。

6. **有効期限は適用したときにしか確かめない**【確認】
   - `couponService.ts:27` の 1 か所だけ。確定時には確かめないので、期限が切れた後に確定しても割引が効く。
   - これが仕様なのかは要確認。

### 新しく生まれた約束
- **【確認】`discountAmount` が必須になった。** 「使っていなければ 0」（`head/src/orders/order.ts:10-12`）。
  - Order を作る箇所はすべて設定が必要になる。
  - SQLite の行は `DEFAULT 0` で保証されるが、InMemory の既存データは型でしか保証されない。
- **【確認】請求額 = `totalAmount - discountAmount`。** 定義しているのは `orderService.ts:44` だけ。
  - 照合バッチは金額を確かめない。課金が見つかれば `PAID` にする（`reconcilePendingPayments.ts:24-26`）。
- **【確認】POST /orders/:id/coupon と応答の割り当て。**
  - 404 は注文がない場合とクーポンがない場合の 2 つで、`error` の値で区別する（`head/src/handlers/applyCoupon.ts:179-183`）。
  - 410 は期限切れ、409 は状態が不正か使用回数の上限。

### 消された行・書き換えられたテスト
- **【確認】`amount: order.totalAmount` が消えた**（diff 209）。この行は「キーと金額が 1 対 1」を暗黙に守っていた（上の 1 番）。
- **【確認】テストの書き換えはフィールドを足しただけ。** 消えたテストはない。
- **足りないテスト**【確認】:
  - 割引後にもう一度確定するテスト（`PAYMENT_FAILED` からの再課金）
  - 同じ注文に 2 回適用するテスト
  - 0 円の課金のテスト
  - `discount_amount` が負の値のテスト

### 同時実行・二重実行・途中失敗・古いコードとの共存
- **同じクーポンを同時に使う:** 上限は守られる【確認】。
- **同じ注文に同時に適用する:** 使用回数が 2 つ減り、後から書いた方が残る【推測】。
- **途中で失敗する:** 使用回数を加算した後に save が失敗すると、回数が戻らない【確認】（上の 5 番）。
- **新旧のコードが同時に動く間**【推測】:
  - 古いコードの upsert は新しい列を UPDATE SET に含まないので、クーポンの情報は残る（`base/src/orders/sqliteOrderRepository.ts:44-49`）。
  - ただし古いコードは `totalAmount` で課金する。クーポンを付けた注文を古いインスタンスが確定すると、満額が課金される。
  - 古いコードは `SELECT *` で余った列を無視するので、エラーにはならない。

### その他
- **不正な JSON で 500 が返る**【確認】: `readJson` の `JSON.parse` が投げる例外が 400 にならず、500 になる。本文の大きさにも上限がない（`head/src/server.ts:280-284`、`server.ts:300-302`）。
- **リポジトリの慣習から外れている**【確認】: `CouponService` は interface ではなく `SqliteCouponRepository` の具象型に依存している（`couponService.ts:5,8,12`）。InMemory 実装もない。
- **README が更新されていない**【確認】: `head/README.md` にクーポンの API も `src/coupons/` も書かれていない。

## 3. 推測のうち確認が必要なもの
- **キーが同じで中身が違う課金:** 決済サービスがどう扱うか（前回の結果を返すのか、エラーか）。拒否された結果もキャッシュされるのか。
- **0 円の課金:** 決済サービスが受け付けるか。
- **本番の構成:** 複数インスタンスで動かすか。動かすなら上の 4 番の競合が現実に起きる。
- **仕様の意図:**
  - 有効期限を適用時だけで判定するのは意図どおりか。
  - 使わなかった分を戻さないのは意図どおりか。
  - 1 つの注文に何度も適用できるのは意図どおりか。
