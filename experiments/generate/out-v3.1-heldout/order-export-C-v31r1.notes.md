**1. 最初の仮説**
- 経理や問い合わせ対応の担当者が、DB を直接見なくても、期間を指定して注文を CSV で取り出せるようにする変更。管理者だけが呼べる、という約束も付いている（commits.txt）。

**2. 変更前のコードが守っている約束（base）**
- `findByStatusUpdatedBefore(status, before)` を使うのは照合バッチだけ。用途は「決済中のまま古くなった注文」を拾うこと（`base/src/jobs/reconcilePendingPayments.ts:17-20`）。型で status が必須になっているので、状態を問わない検索はできない（`base/src/orders/orderRepository.ts:6`）。
- 索引は `(status, updated_at)` で、コメントには照合バッチのためとある（`base/db/migrations/001_create_orders.sql:11-12`）。
- `updated_at` は保存のたびに上書きされる「最後に更新した時刻」で、呼び出し側から渡された値は使わない（`base/src/orders/sqliteOrderRepository.ts:51`、InMemory は `:24`）。
- 注文は CART → PENDING_PAYMENT → PAID / PAYMENT_FAILED と進み、PAYMENT_FAILED からはやり直せる（`base/src/orders/orderService.ts:33-40`）。
- 冪等キーは必ず `paymentKeyFor` で作る決まり（`base/src/orders/order.ts:13-19`）。金額は円の整数（migration のコメント）。
- 設定の既定値は app.ts の `??` で決めている。たとえば PAYMENT_API_KEY には `'test-key'` が入る（`base/src/app.ts:10-11`）。
- 例外は handler で HTTP の結果に変え、server.ts で 500 にする（`base/src/handlers/checkout.ts`、`base/src/server.ts:14-20`）。

**3〜4. 8 つの分野を当てた結果**
1. 約束：兆しあり。メソッドの型が `status | null` に広がり、名前と中身がずれた。`updated_at` の意味（最後の更新）と、「期間内の注文」という要望がずれている。照合バッチの振る舞いは変わっていない（fact：条件と引数の順を確かめた）。
2. 失敗：兆しあり。ADMIN_TOKEN が未設定だと `undefined !== undefined` が false になり、認可が通す側に倒れる（`head/src/handlers/orderExport.ts:26`、`head/src/app.ts:15`）。
3. 出したあと：読み取り専用で、新しい入口を足しただけなので戻せる。ただし外に出たデータは戻せない。誰が書き出したかのログもない。
4. 信頼の境界：兆しあり（上の 2 と同じ認可の問題）。SQL は固定の文字列を連結し、値はバインドしているので問題ない。ファイル名の from / to は検証を通ったあとの値なので、ヘッダーへの差し込みはできない。
5. 規模：軽い兆し。status を指定しない検索は `(status, updated_at)` の索引の先頭の列を使えない（推測。SQLite の計画は確かめていない）。期間に上限がなく、CSV 全体を文字列としてメモリに持つ。
6. 次に変える人：兆しあり。1 つのメソッドに、呼び出し元ごとの引数（null と省略できる since）が増えた。`(null, to, from)` という to と from の逆順を、型では防げない。
7. テスト：兆しあり。カンマのテストは期待値を本体と同じ `toCsvRow` で作っているので、何も証明していない。トークン未設定の場合を試すテストがない。SQLite で status を絞る検索は、変更の前も後もテストされていない。
8. AI 特有：`URL.parse` と `node:sqlite` はリポジトリの外なので確かめられない（README には Node 23.6 以降とある）。頼まれていない抽象は足していない。差分の範囲も適切。

**5. 見つけたこと**
- 必須：ADMIN_TOKEN が未設定のとき、トークンなしで全注文が返る。
- 提案：`findByStatusUpdatedBefore` を共通化せず、`findUpdatedBetween(from, to)` を別に作る。
- 問い：経理の月次で欲しいのは「最後の更新が期間内の注文」でよいか（PAYMENT_FAILED からやり直すと、前の月の CSV から注文が消える）。
- 提案：カンマのテストの期待値を文字列で直接書く。トークン未設定のテストを足す。
- 提案：書き出した記録をログに残す。
- 問い：status を指定しない検索の計画（索引を使えるか）。
- Nit：トークンの比較を一定時間で行う。Excel で開くなら BOM を付けるかと、数式として解釈される値（CSV インジェクション）への対策。InMemory は並び順を保証していない。
- 修正案 `!deps.adminToken ||` は、値を当てて確かめた：undefined / undefined → 401、'' / '' → 401、secret / secret → 200、secret / undefined → 401。

**6. 手順を厚く使うところ**
- 認可が通す側に倒れる問題：損害が戻せず、一番重い。
- 共通化：次に変える人への影響と、照合バッチへの波及がある。
- `updated_at` の意味：仮説を直すところ。
- テストの空振り：ジュニアが見落としやすい。
- 良い点：日本時間で区切る半開区間、実在しない日付の弾き方、`paymentKeyFor` の再利用、値のバインド。
- 気にしなくてよいもの：SQL インジェクション、ファイル名ヘッダー、戻せるかどうか。
