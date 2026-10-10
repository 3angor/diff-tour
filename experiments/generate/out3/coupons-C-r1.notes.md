**1. 最初の仮説**
- 買い物客が注文確定の前にクーポンコードを入れると割引が付き、割り引いた額で課金される。仮説として「クーポンは 1 つの注文に 1 回だけ効き、使用回数は 1 注文につき 1 回だけ減る」と置いて、これを確かめる。

**2. 変更前のコードが守っている約束（base）**
- `PENDING_PAYMENT` は「決済中」の意味。課金の前に保存するので、連打や処理中の 2 回目は状態の確認で弾かれる（`src/orders/orderService.ts:37-40`）。これはアプリの手順で守っている。
- 冪等キーは注文 ID だけで作る（`src/orders/order.ts:13-19`）。同じキーの課金は決済サービス側で 1 回分として扱われる（`src/payment/gateway.ts:6-7`）。カードが拒否されたら、同じキーで再挑戦できる（テスト `test/orderService.test.ts:37-43`）。
- 金額は円の整数で持ち、負の値は DB の CHECK で防ぐ（`db/migrations/001_create_orders.sql:4-5`）。
- 外部キーは `db.ts:10` の `PRAGMA foreign_keys = ON` で有効になっている。
- `save` は行全体を上書きする upsert（`sqliteOrderRepository.ts`）。
- リポジトリにはインターフェースと InMemory 実装がある（`orderRepository.ts`）。

**3. 変更と約束の突き合わせ（head）**
- `apply` は checkout と同じ状態の確認を使う（`couponService.ts:21`）。PENDING の注文に割引を付け替えることはできない。約束は守られている。
- `consumeUse` は確認と加算を 1 つの UPDATE で行う（`sqliteCouponRepository.ts:35-39`）。上限を超えないことは DB の操作 1 回で守っている。良い点。
- 一方、`apply` は注文にすでにクーポンが付いているかを見ない。同じ注文で何度呼んでも毎回 `used_count` が増え、付け替えると前のクーポンの回数は戻らない（`couponService.ts:25-33`）。
- `consumeUse` と `orders.save` はトランザクションで囲まれていない。

**4. 8 分野の当てはめ**
1. 約束: 兆しあり。使用回数の意味がずれる（2 回の適用で 2 回分減る）。`orders.discount_amount` には CHECK がなく、`0 ≤ 割引 ≤ 合計` はアプリの `Math.min` だけで守っている（head `002_add_coupons.sql:10`、`couponService.ts:32`）。`expires_at` は TEXT で形式の制約がない。
2. 失敗: 兆しあり。
   - 期限の文字列が壊れていると `Invalid Date` になり、`<=` が false になるので期限切れにならない（通す側に倒れる）。
   - 割引が全額だと 0 円で課金する。0 円を決済サービスがどう扱うかはリポジトリの外で、分からない。
   - consumeUse のあと save が落ちると、回数だけ減る。
3. 出したあと: ほぼ十分な場合に当たる。列の追加だけで、既定値があり、SQLite の ADD COLUMN。ただし `used_count` のずれは後から直しにくいデータになる。README が更新されていない。
4. 信頼の境界: 十分な場合に当たる。code は SQL のプレースホルダで渡している。注文の持ち主の確認がないのは base の checkout も同じ（今回の範囲外）。
5. 規模: 兆しなし。1 回の要求で行を 1 つ読み書きするだけ。
6. 次に変える人: 小さい兆し。CouponService が具体クラス SqliteCouponRepository に依存しており、orders の慣習（インターフェースと InMemory）から外れている。
7. テスト: 兆しあり。`couponService.ts:28` や `orderService.ts:44` を壊せばテストは落ちる。しかし「同じ注文に 2 回」「付け替え」「0 円」「壊れた期限」のテストはない。既存テストの変更は `discountAmount: 0` を足しただけで、期待値は変わっていない（fact）。
8. AI 特有: 存在しない API の呼び出しはない（`node:sqlite` の `changes` は既存コードの流儀どおり）。頼まれていない抽象もない。

**5. 選ぶ分野**
- 1・2・7（使用回数の意味、端の値、失敗したときの倒れ方）。お金とデータのずれに関わり、後から直しにくいので選ぶ。
- 問いとして残すもの: `PAYMENT_FAILED` のあとに割引を付けると、同じ冪等キーで金額だけが変わる。決済サービスがこれをどう扱うかは外の話。
- 気にしなくてよいもの:
  - apply と checkout の競合。同期の SQLite と Node の単一スレッドで、apply の中に別の要求が割り込めない（inferred）。checkout は課金前に PENDING を保存するので、課金中の apply は弾かれる（fact）。
  - 認可がないこと。base から同じ。
- 良い点: consumeUse の 1 文の UPDATE、状態の確認の再利用、外部キーが本当に効いていること、割引の上限を付けたこと、checkout のテストを足したこと。
