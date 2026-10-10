# クーポン機能の差分（base → head）レビュー前の下調べ

**要点：** 一番重く見るべきなのは次の 4 つです。
- クーポンの使用回数を、注文を確定するときではなくクーポンを当てるときに消費している。しかも 1 つの注文に何度でも当てられる。
- 割引額と使用回数の範囲を、データベースの制約で守っていない。base は金額を CHECK 制約とテストで守っており、その慣習から外れている。
- 決済に失敗した注文の金額を、同じ冪等キーのまま変えられるようになった。
- 割引額が注文金額と同じとき、0 円で課金する。

実行時に確かめたものはありません。下の「確定」はコードを読んで確かめたこと、「推測」は外部の決済サービスの挙動やデプロイの方法に依存することです。

---

## 1. 変更前（base）のコードが守っている約束

### 状態と値の意味
- **注文の状態は 4 つ**（`CART` / `PENDING_PAYMENT` / `PAID` / `PAYMENT_FAILED`）。型（`base/src/orders/order.ts:1`）とデータベースの CHECK（`base/db/migrations/001_create_orders.sql:6`）の両方で守っている。
  - `PENDING_PAYMENT` の意味は「課金されたか分からない」。決済サービスの障害やタイムアウトのときは、この状態のまま残す（`base/src/orders/orderService.ts:56-58`）。
  - `PAYMENT_FAILED` の意味は「課金の記録がないので、やり直しても二重課金にならない」（`base/src/jobs/reconcilePendingPayments.ts:28`）。
- **金額は円の整数で、0 以上**（`001_create_orders.sql:4-5`）。
  - 「REAL にすると誤差が出る」という理由がコメントに書いてある。
  - 0 以上であることはデータベースの CHECK で守っており、テストもある（`base/test/sqliteOrderRepository.test.ts:14-20`）。
  - つまり、金額の範囲は**アプリの手順ではなくデータベースの制約で守る**のがこのコードの慣習。
- **課金額は `order.totalAmount` そのもの**（`base/src/orders/orderService.ts:44`）。

### 不変条件（何回処理されてよいか）
- **1 つの注文の課金は 1 回だけ。** 冪等キーは注文 ID から作り（`order-${id}`）、注文確定と照合バッチが同じキーを使う。キーは必ず `paymentKeyFor` で作るという決まりがある（`base/src/orders/order.ts:13-19`）。
  - 決済サービス側は、同じキーの課金を 1 回分として扱う（保持期間 24 時間、`base/src/payment/gateway.ts:6`）。
  - テストあり：`base/test/orderService.test.ts:23-27`。
  - 暗黙の前提として、**同じキーで送る課金の内容（金額）は毎回同じ**になる。base には確定前に金額を変える経路がない。
- **二重の確定を防ぐ仕組み。** 課金の前に `PENDING_PAYMENT` を保存し、2 回目の確定は状態チェックで弾く（`orderService.ts:33-40`）。
  - これは**アプリの手順**で守っており、比較して書き換える（CAS）ような条件付き更新ではない。
  - 守れているのは、SQLite が同期 API で、`get` から `save` までの間に別のリクエストが割り込まないから（推測）。
- **照合バッチは、更新から 10 分以上たった `PENDING_PAYMENT` だけを扱う**（`reconcilePendingPayments.ts:5-6`）。テストあり：`base/test/reconcilePendingPayments.test.ts:24-29`。

### 失敗したときの扱い
- **カード拒否**：`PAYMENT_FAILED` にする。やり直せる（テスト `orderService.test.ts:29-35`）。
- **タイムアウト・5xx・接続失敗**：`PENDING_PAYMENT` のまま残す（テスト `:37-41`）。
  - HTTP では 503 `payment_pending` を返す（`base/src/handlers/checkout.ts:24-31`）。
- **想定外の HTTP ステータス（400 など）**：ただの `Error` になる（`base/src/payment/httpGateway.ts:35-37`）。
  - ハンドラはこれを投げ直し、サーバーが 500 を返す（`base/src/server.ts:17-19`）。注文は `PENDING_PAYMENT` のまま残る。

### 接続と初期化で決まる振る舞い
- `openDatabase` で **`PRAGMA foreign_keys = ON`** にしている（`base/src/db.ts:10`）。外部キーの宣言が効くのはこの経路で開いた接続だけ。
- マイグレーションは起動時に番号順で 1 ファイルずつ当て、1 ファイルを 1 トランザクションにしている（`db.ts:15-26`）。
- `save` は全列を上書きする upsert（`base/src/orders/sqliteOrderRepository.ts:39-52`）。
  - 呼び出し側が読んだ時点の Order オブジェクトで、行をまるごと書き戻す。後勝ちになる。

### 慣習
- リポジトリは「interface ＋ InMemory 実装 ＋ Sqlite 実装」の形にしている（`base/src/orders/orderRepository.ts:3-32`）。
- 業務エラーはクラスで投げ、ハンドラで HTTP ステータスに変換する（`checkout.ts`）。
- README には API とディレクトリ構成を書いている（`base/README.md:7, 12-17`）。
- 型チェック（tsc）は npm scripts に入っていない（`base/package.json:5-8`）。Node が型を消してそのまま実行するだけなので、型の誤りは実行時には捕まらない（確定）。

---

## 2. 変更（head）と約束の突き合わせ

### 頭を使うべき変更
- `002_add_coupons.sql`
- `couponService.ts`、`sqliteCouponRepository.ts`
- `orderService.ts:44` の課金額
- `server.ts` の body 読み取りと振り分け
- `applyCoupon.ts` のステータス対応

### 機械的な変更
- 既存のテスト 3 ファイルに `discountAmount: 0` を足しただけ。アサーションは消えておらず、base のテストが守っていた振る舞いは残っている（確定）。
- `sqliteOrderRepository.ts` に列を足した（Row、toOrder、save）。
- `app.ts` の組み立て。

### 守られている約束
- **既存の注文**：`discount_amount DEFAULT 0` なので、マイグレーション後の既存の行は 0 になる。課金額は従来どおり（確定、`head/db/migrations/002_add_coupons.sql:10`）。
- **外部キー**：`coupon_code REFERENCES coupons(code)` は、`db.ts:10` の PRAGMA で実際に効く（確定）。
- **使用回数の上限**：`consumeUse` は確認と加算を 1 つの UPDATE で行うので、同時に使われても上限を超えない（確定、`head/src/coupons/sqliteCouponRepository.ts:35-40`）。
- **確定を始めた注文には当てられない**：`apply` も `CART` / `PAYMENT_FAILED` 以外を弾く（`head/src/coupons/couponService.ts:20-22`）。1 プロセスの SQLite であれば、`PENDING_PAYMENT` 以降の注文にクーポンは入らない（推測。base の二重確定防止と同じ前提に頼っている）。
- **照合バッチ**は変更されていない。`PENDING_PAYMENT` しか触らないので、クーポンの処理とは干渉しない（確定）。

### 破られている約束、または危うい約束
1. **同じキーで違う金額を送るようになった**（`couponService.ts:20` が `PAYMENT_FAILED` での適用を許している）。
   - カード拒否や照合バッチで `PAYMENT_FAILED` になった注文にクーポンを当てると、同じ冪等キー `order-<id>` のまま金額だけが変わって再送される（確定）。
   - 24 時間の保持期間内なら、決済サービスがキーの使い回しとしてエラーにするか、前回の結果を返す可能性がある（推測。サービスの仕様を確認すべき）。
   - エラーが 400 系なら `httpGateway.ts:35-37` でただの Error になる。その結果、500 が返り、注文は `PENDING_PAYMENT` に残り、照合バッチがまた `PAYMENT_FAILED` に戻す、という繰り返しになりうる（推測）。
2. **割引額の範囲を制約で守っていない。** base は金額の範囲を CHECK とテストで守っていたが、head はそうしていない。
   - `orders.discount_amount` に CHECK がない（`002_add_coupons.sql:10`）。
   - 課金額 `totalAmount - discountAmount` が 0 以上になることは、適用したときの `Math.min` だけで守っている（`couponService.ts:32`）。
   - 適用した後に `total_amount` が書き換わると、課金額がマイナスになりうる。確定時（`head/src/orders/orderService.ts:44`）にはもう一度確かめていない。いまのリポジトリに注文金額を変えるコードはないので、今後の経路の話（確定）。
   - `CHECK (discount_amount >= 0 AND discount_amount <= total_amount)` のような制約にできないか。ADD COLUMN でほかの列を参照する CHECK を付けられるかは要確認（推測）。
   - `coupons.used_count` にも `CHECK (used_count BETWEEN 0 AND max_uses)` がない。上限はアプリの UPDATE 文だけで守っている。
3. **0 円の課金。** 割引額が注文金額と同じなら `amount: 0` で `charge` を呼ぶ。テスト「割引額は注文金額を超えない」（`head/test/couponService.test.ts:42-46`）がまさにこの状態を作るが、確定まではテストしていない。
   - 決済サービスが 0 円の課金を拒むなら、上の 1 と同じ経路で 500 になる（推測）。
4. **1 つの注文に何回でも当てられる**（確定）。
   - `apply` は `order.couponCode` がすでに入っているかを見ない（`couponService.ts:18-35`）。
   - 同じ注文に同じコードを 2 回当てると、使用回数を 2 つ消費する。
   - 別のコードに付け替えると、前のクーポンの使用回数は戻らない。
   - 1 注文 1 回を制約で守る形（たとえば使用記録のテーブルに UNIQUE(order_id) を置く）も検討の余地がある。
5. **使用回数を当てた時点で消費し、トランザクションもない**（確定）。
   - `consumeUse` が成功したあと `orders.save` までの間に例外が出たりプロセスが落ちたりすると、使用回数だけが減る（`couponService.ts:28-33`）。`db.exec('BEGIN')` などで包んでいない。
   - 確定しなかったカートや決済失敗でも、使用回数は戻らない。これが仕様かどうかは要確認。
6. **有効期限を手順で確かめており、不正な値だと期限切れにならない。**
   - 期限の確認は SELECT のあとアプリで行い、UPDATE の条件には入っていない（`couponService.ts:27-28`）。
   - `expires_at` は TEXT で、形式の制約がない（`002_add_coupons.sql:6`）。日付として解釈できない値だと `new Date()` が Invalid Date になり、`Invalid Date <= now` は false なので、**期限切れにならない**（確定。JS の NaN 比較による）。
   - テストでは `'...T00:00:00Z'` 形式の値を入れている。base は `toISOString()` の `'.000Z'` 形式で書き、文字列のまま比べている（`base/.../sqliteOrderRepository.ts:56`）。もし SQL 側で期限を比べるように変えると、この形式の違いで結果がずれる（推測）。
7. **新旧のコードが同時に動く場合**（デプロイの方法が不明なので推測）。
   - 新しいプロセスがマイグレーションを当てたあとも古いプロセスが動いていると、古い `checkout` は割引を無視して全額を課金する。
   - 古い `save` は新しい列を UPDATE しないので、値は消えない（確定、base の `sqliteOrderRepository.ts:44-49`）。
8. **JSON の解析に失敗すると 500 になる**（確定）。
   - `readJson` の `JSON.parse` が投げると、外側の catch で 500 になる。400 `code_required` にはならない（`head/src/server.ts:9-13, 27-30`）。
   - body の大きさに上限がない（`server.ts:11`）。

### 新しく生まれた約束
- `Order.discountAmount` を必須にし、「クーポンなしなら 0」とした（`head/src/orders/order.ts:10-12`）。
  - 型でしか強制していないので、実行時は tsc なしでも動く（`package.json` に型チェックがないのは確定）。
- `orders.coupon_code` は `coupons.code` を参照する（外部キーで守っている）。
- 課金額は `totalAmount - discountAmount`（`orderService.ts:44`）。
- `POST /orders/:id/coupon` の応答（`head/src/handlers/applyCoupon.ts:8-19`）：
  - 400：コードがない
  - 404：注文がない、またはクーポンがない（エラー名で区別）
  - 409：注文の状態が不正、または使用回数の上限
  - 410：期限切れ

### 慣習から外れているところ
- **リポジトリに interface がない。** `CouponService` は具体的な `SqliteCouponRepository` に依存しており、interface も InMemory 実装もない（`couponService.ts:5, 8, 12`）。注文側の慣習（`orderRepository.ts`）と違う。
- **README を更新していない。** 新しい API も `src/coupons/` も書かれていない（`head/README.md:7, 12-17`、base と同じ内容）。
- **テストが足りない。** 次のテストがない。
  - 同じ注文への再適用、付け替え
  - `PAYMENT_FAILED` の注文への適用
  - 割引で 0 円になったときの確定
  - 割引額を SQLite で保存して読み出すこと
  - 不正な `expires_at`
