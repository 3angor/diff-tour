**最初の仮説**
- 客が `POST /orders/:id/coupon` でコードを送ると注文に割引が付き、注文確定では割り引いた額で課金される。暗黙の前提は「1 回の利用は 1 つの注文に対応し、合計は `max_uses` を超えない」。

**変更前のコードが守っている約束（base）**
- 金額は円の整数で持つ。理由はコメントにある（base `db/migrations/001_create_orders.sql:4-5`）。`total_amount >= 0` は DB の CHECK で守っている。
- 連打への備えは状態で守っている。checkout は先に PENDING_PAYMENT を保存し、2 回目は状態チェックで弾く（base `src/orders/orderService.ts:37-40`）。
- 冪等キーは注文 ID だけから作る `order-<id>`（`src/orders/order.ts:20-22`）。保持期間は 24 時間（`src/payment/gateway.ts:6`）。base には注文金額を書き換えるコードがないので、「1 つのキーに金額は 1 つ」が成り立っていた。
- 外部キーが効くのは `PRAGMA foreign_keys = ON` があるから（`src/db.ts:10`）。マイグレーションは起動時に番号順に当てる。
- 照合バッチは金額を使わない（`src/jobs/reconcilePendingPayments.ts:22-32`）。

**変更との突き合わせ**
- checkout で変わったのは課金額の 1 行だけ（head `src/orders/orderService.ts:44`）。状態の約束はそのまま。
- `discountAmount` を必須の型にしたので、Order を作る場所は全部コンパイラが直させる。テストの差分はほぼ `discountAmount: 0` を足しただけの機械的な変更。
- `consumeUse` は確認と加算を 1 つの UPDATE で行う（head `src/coupons/sqliteCouponRepository.ts:35-40`）。
- ただし `apply` は、注文にもうクーポンが付いているかを見ていない（head `src/coupons/couponService.ts:18-35`）。checkout にある連打への備えに当たるものもない。

**8 分野の当てはめ**
1. 約束：**兆しあり**。同じ注文に 2 回 apply すると `used_count` が 2 増える。別のコードに替えても、前のクーポンの回数は戻らない（`couponService.ts:28,30`）。`expires_at` は TEXT で形式の決まりがなく、壊れた値だと「期限切れでない」側に倒れる（`sqliteCouponRepository.ts:27`、`couponService.ts:27`。推測）。
2. 失敗：**兆しあり（軽い）**。`consumeUse` と `save` が別々の文で、トランザクションに入っていない。同時実行は、同期の DB とマイクロタスクのため 1 プロセスなら割り込まれにくい（推測）。
3. 出したあと：**十分な場合に当たる**。列を足すだけで既定値は 0。古いコードが書いても既定値が入る。ただし PAYMENT_FAILED の後でも apply でき、同じ冪等キーのまま金額が変わる。決済サービスがこれをどう扱うかはリポジトリからは分からない（問い）。
4. 信頼の境界：**兆しはあるが今回の範囲外**。注文の持ち主の確認は既存の checkout にもない。コードは型だけ確かめてから SQL に bind しているので安全。
5. 規模：**十分な場合に当たる**。1 要求あたり定数回のクエリ。
6. 次に変える人：**Nit**。`CouponService` が具体型の `SqliteCouponRepository` に依存していて、OrderRepository の作り方（interface）とずれる。不正な JSON が 500 になる。
7. テスト：**兆しあり**。肝心の行を壊せば既存のテストが落ちる。ただし「同じ注文に 2 回」を試すテストがない。どのテストも本物の SQLite を使っていて、mock ではない。
8. AI 特有：**兆しなし**。`node:sqlite` の `changes` はリポジトリの外のものなので確かめられない。頼まれていない抽象は足していない。

**選んだ分野と理由**
- 1 と 7（使用回数の約束が手順で破れる）を厚く扱う。お金と上限が絡み、`used_count` が一度増えると戻す手段がない。
- 3 の冪等キーと 0 円の課金は問いとして出す。
- 良いところ：1 文の UPDATE、`Math.min`、型での必須化、テストの期待値が直書きであること。
- 気にしなくてよいもの：同時実行（今は）、認可（既存と同じ）、マイグレーションのロック、規模、照合バッチ。
