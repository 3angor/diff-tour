**変更前（base）が守っている約束**
- 注文の状態：`CART` と `PAYMENT_FAILED` だけが確定できる。課金の前に `PENDING_PAYMENT` を保存し、連打を状態チェックで弾く。途中で落ちても照合バッチが拾う（base `src/orders/orderService.ts:33-40`）。
- 課金の冪等キーは `order-${id}` だけで作る。確定処理と照合バッチの両方が同じキーを使う（base `src/orders/order.ts:13-19`）。同じキーの課金は決済サービス側で 1 回分として扱われ、保持期間は 24 時間（base `src/payment/gateway.ts:6`）。
- 拒否されたら `PAYMENT_FAILED` にして、同じキーでやり直せる。この振る舞いはテストが守っている（head `test/orderService.test.ts:37-43`。このテストは変更なし）。
- 金額は円の整数で持ち、`total_amount >= 0` を DB の CHECK で守る（base `db/migrations/001_create_orders.sql:4-5`）。テスト `金額が負の注文は保存できない` がこれを守る。
- 接続：`PRAGMA foreign_keys = ON` で外部キーが効く（base `src/db.ts:10`）。マイグレーションは起動時に番号順に当て、1 ファイルずつトランザクションにくるむ（base `src/db.ts:15-26`）。
- 慣習：リポジトリはインターフェース（`OrderRepository`）と InMemory 版・SQLite 版の 2 つの実装に分ける（base `src/orders/orderRepository.ts:3-32`）。README は構成と API を書いている（`README.md:7`）。
- 保存は全列の upsert。読み出しはコピーを返す（InMemory の `structuredClone`）。

**変更（head）との突き合わせ**
- クーポンの使用回数は、条件付きの UPDATE 1 文で「上限の確認と加算」を行う（head `src/coupons/sqliteCouponRepository.ts:31-40`）。正しい。
- `apply` は注文にすでにクーポンが付いているかを見ない（head `src/coupons/couponService.ts:18-35`）。同じ注文に 2 回呼ぶと使用回数が 2 つ減る。別のクーポンに付け替えると、前のクーポンの 1 回分が戻らない。（fact）
- `consumeUse` と `orders.save` は 1 つのトランザクションに入っていない（head `couponService.ts:28-33`）。間で落ちると使用回数だけが減る（inferred）。db.ts には BEGIN/COMMIT の前例がある。
- `PAYMENT_FAILED` の注文にもクーポンを付けられる。そうすると、同じ冪等キーで前回と違う金額を送ることになる（inferred。決済サービス側の振る舞いは渡されたファイルからは分からない）。
- 割引額が注文金額と同じなら 0 円の課金を送る。テストもこの状態を作っている（head `test/couponService.test.ts:35-39`）。0 円を決済サービスが受け付けるかは不明。
- `orders.discount_amount` には CHECK がない。`expires_at` の形式も DB で検証しない。形式の壊れた日時を入れると `Invalid Date` との比較が常に false になり、そのクーポンは期限切れにならない（inferred）。
- 同時実行：DatabaseSync は同期 API。`apply` の中の await は I/O を待たないので、他のリクエストに割り込まれない（inferred）。確定処理の課金中は状態が `PENDING_PAYMENT` なので、`apply` は 409 になる。
- 慣習とのずれ：CouponService は具象クラスの `SqliteCouponRepository` に依存している。`InvalidOrderStateError` の文言が「cannot be checked out」のまま流用されている。README に新しい API がない。JSON が壊れていると 500 を返す（head `src/server.ts:9-13, 27-30`）。
- 機械的な変更：既存テストの `discountAmount: 0` の追加（5 か所。assert は変わらない）、`app.ts` の組み立て、`sqliteOrderRepository` の列追加。InMemory 版は `structuredClone` で新しい項目もそのまま運ぶ。照合バッチも割引を消さない。
- 新旧の共存：既存の行は `DEFAULT 0` で割引なしになる。古いコードが新しいスキーマで動くと割引を無視するが、プロセスは 1 つで起動時にマイグレーションを当てるので、影響は小さい（inferred）。
