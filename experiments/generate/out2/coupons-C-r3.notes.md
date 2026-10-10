下調べが終わったので、ここからツアーの JSON を書く。


- **変更前の約束（base）**
  - 注文確定は「状態の確認 → PENDING_PAYMENT を保存 → 課金」の順で進む。連打で 2 回目が来ても状態の確認で弾き、課金中にプロセスが落ちたら照合バッチが拾う（`src/orders/orderService.ts:37-40`）。
  - 冪等キーは注文 ID だけから作る（`order-<id>`）。注文確定と照合バッチが同じキーを使う（`src/orders/order.ts:13-19`）。決済サービスは同じキーの課金を 1 回分として扱い、24 時間保持する（`src/payment/gateway.ts:6`）。
  - カードが拒否されたら PAYMENT_FAILED になり、同じキーで確定をやり直せる。テストがこれを守っている（`test/orderService.test.ts:29-35`）。このとき課金額は変わらなかった。
  - 金額は円の整数で持ち、負にならないことは DB の CHECK で守る（`001_create_orders.sql:4-5`）。テスト「金額が負の注文は保存できない」もこれに頼っている。
  - `PRAGMA foreign_keys = ON` は `db.ts:10` で有効になっている。マイグレーションはファイルごとに BEGIN/COMMIT で当てる（`db.ts:17-25`）。このリポジトリでトランザクションを書く手本はここにある。
  - リポジトリは interface と InMemory 実装に分けている（`orderRepository.ts:3-7`）。README には構成と API が書いてある。
  - `npm test` は `node --test` で型を剥がして実行するだけで、tsc を通さない（`package.json:5-8`）。型を必須にしても、テストの実行時には確かめられない。
- **変更との突き合わせ（head）**
  - consumeUse は確認と加算を 1 つの UPDATE で行う（`sqliteCouponRepository.ts:35-40`）。上限を超えないことは、この UPDATE の条件で守られている。
  - apply には「同じ注文に既にクーポンが付いているか」の確認がない（`couponService.ts:18-35`）。同じ注文に 2 回適用すると used_count が 2 増え、差し替えた古いクーポンの回数も戻らない。テストは o1 と o2 という別々の注文でしか試していない（`test/couponService.test.ts:46-51`）。
  - consumeUse と save がトランザクションでまとまっていない。save で落ちると回数だけが消費される。期限の確認も UPDATE の外で行っている。
  - PAYMENT_FAILED の注文にもクーポンを付けられる。そのため同じ冪等キーで別の金額を送れるようになった。これは既存の約束を変えている。決済サービスがどう振る舞うかはファイルからは分からない。
  - `orders.discount_amount` にも `coupons.used_count` にも CHECK がない。001 の慣習から外れている。
  - CouponService が SqliteCouponRepository という具体的な型に依存している。interface の慣習から外れている。README も更新されていない。
  - `expires_at` は TEXT で、形式を DB で縛っていない。不正な値だと Invalid Date になり、比較が false になって期限切れと判定されない（推測）。
  - JSON として読めない本文は 500 になる（`server.ts:12, 27-30`）。
  - 外部キーは有効なので、coupon_code の REFERENCES は効いている。
  - 入れ替えの途中で古いコードが動くと、割引を無視して満額を課金する（推測）。
  - 機械的な変更: テストへの `discountAmount: 0` の追加、Row や upsert への列の追加、app.ts の組み立て。振る舞いは変えていない。
