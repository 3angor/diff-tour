**変更前（base）が守っている約束**
- 注文の状態の意味：`CART`/`PAYMENT_FAILED` のときだけ確定できる。課金の前に `PENDING_PAYMENT` を保存し、連打や途中で落ちた場合に備えている（base `src/orders/orderService.ts:33-40`）。
- 失敗の扱い：カードの拒否なら `PAYMENT_FAILED`、障害やタイムアウトなら `PENDING_PAYMENT` のまま残し、照合バッチに任せる（base `orderService.ts:51-58`、`jobs/reconcilePendingPayments.ts:22-31`）。
- 冪等キーは注文 ID だけから作る（base `src/orders/order.ts:13-19`）。「同じキーで送られた課金は 1 回分として扱われる（保持期間 24 時間）」（base `src/payment/gateway.ts:6`）。base では注文の金額が変わらないので、同じキーは常に同じ内容の課金を指していた。
- 金額は円の整数で持ち、`CHECK (total_amount >= 0)` で DB が守る（base `db/migrations/001_create_orders.sql:4-5`）。テスト「金額が負の注文は保存できない」もこれを守っている。
- 接続と設定：`openDatabase` が `PRAGMA foreign_keys = ON` を有効にし、起動時にマイグレーションを番号順に、トランザクションの中で当てる（base `src/db.ts:10,15-26`）。
- 慣習：`OrderService` はインターフェース `OrderRepository` に依存し、テストでは InMemory 版を使う（base `orderRepository.ts:3-7`）。README には API と構成が書いてある（`README.md:7,12-17`）。
- 型チェックは `npm test` に含まれない。Node は型を消して動かすだけ（`package.json:5-7`）。

**変更と突き合わせた結果（head）**
- 既存の約束：冪等キーは注文 ID のままなのに、`PAYMENT_FAILED` でもクーポンを付けられる（`couponService.ts:21`）。このため同じキーで違う金額を送りうる（`orderService.ts:44,47`）。base の決済テストは削除も書き換えもされていない。足されたのは `discountAmount: 0` だけ。
- 二重実行：同じ注文に apply を 2 回呼ぶと使用回数が 2 増える。別のクーポンに付け替えると、前のクーポンの使用回数が戻らない（`couponService.ts:25-33` に `order.couponCode` の確認がない）。
- 途中の失敗：`consumeUse` と `orders.save` がトランザクションに入っていない（`couponService.ts:28-33`）。カートを放置したり決済に失敗したりしても使用回数は戻らない。
- 同時実行：`consumeUse` は条件付きの UPDATE 1 文で、上限を超えないことを守っている（`sqliteCouponRepository.ts:31-39`）。apply と checkout の同時実行は、DB が同期の `DatabaseSync` で await が microtask だけなので、1 プロセスなら割り込まれないと推測する（inferred）。`save` は行全体を上書きするので、非同期のドライバーに替えると状態が CART に巻き戻る危険がある。
- 型や制約：`orders.discount_amount` に CHECK がない。`coupons.used_count <= max_uses` も DB の制約ではなく、アプリの WHERE だけで守っている（`002_add_coupons.sql`）。
- 接続：`REFERENCES coupons` は `db.ts:10` の PRAGMA で効く。既存の行は `DEFAULT 0` になる。
- 新旧の共存：古いプロセスの INSERT は新しい列を書かない（既定値になる）。古い checkout は割引を無視して全額を課金する（inferred）。
- 慣習：`CouponService` は具象クラス `SqliteCouponRepository` に依存している（`couponService.ts:5,8,12`）。README は更新されていない。不正な JSON は 400 ではなく 500 になる（`server.ts:12,21-29`）。割引が全額なら 0 円の課金を送る。
- 機械的な変更：テストの `discountAmount: 0` 追加、`sqliteOrderRepository` の列の追加、`app.ts` の組み立て。アサーションは変わっておらず、振る舞いも変わっていない。
