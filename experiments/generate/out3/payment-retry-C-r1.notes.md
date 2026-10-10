**1. 最初の仮説**
- 決済サービスが一時的に 503 を返すと注文確定が失敗し、利用者は「注文履歴を見に行け」と言われるだけだった。今回の変更は (A) `RetryingPaymentGateway` で自動でやり直す、(B) それでも失敗したら `PAYMENT_FAILED` にしてすぐやり直せるようにする、の 2 つでその困りごとをなくす（commits.txt:3-5）。
- 仮説: B は A の当然の帰結に見える。本当にそうかを確かめる。

**2. 変更前の約束（base）**
- `PaymentTimeoutError` は「届いていて課金済みかもしれない」、`PaymentUnavailableError` は「課金されていない」という意味になっている（base/src/payment/gateway.ts:31, 42）。httpGateway は接続失敗を「届いていない」として Unavailable に分けている（base/src/payment/httpGateway.ts:63-69）。
- 冪等キーは決済サービス側で 24 時間だけ 1 回分として扱われる（gateway.ts:6）。キーは注文 ID から作り、注文確定と照合バッチが同じキーを使う（order.ts:13-19）。
- `PENDING_PAYMENT` は「課金されたか分からない」状態を表す。決済サービスが落ちたときやタイムアウトしたときはこの状態のまま残し、照合バッチに任せる（base/src/orders/orderService.ts:51-58）。`PAYMENT_FAILED` は「課金の記録がないので、もう一度確定しても二重課金にならない」状態を表す（reconcilePendingPayments.ts:28）。
- 照合バッチは 10 分以上前の `PENDING_PAYMENT` だけを見る（reconcilePendingPayments.ts:17-20）。5 分ごとに動く（server.ts:23-25）。
- 連打への備えは、課金の前に `PENDING_PAYMENT` を保存し、状態チェックで弾くという手順で守っている（orderService.ts:33-40）。型や DB の制約で守っているわけではない。
- 既存のテスト「タイムアウトしたら PENDING_PAYMENT のまま残す」がこの約束を守っていた（base/test/orderService.test.ts）。

**3. 突き合わせ（head）**
- head/src/orders/orderService.ts:50-56 は、エラーの種類を問わず `PAYMENT_FAILED` にする。タイムアウト（課金済みかもしれない）もここに入る → 約束 2 を破っている。
- 照合バッチは変わっておらず、`PAYMENT_FAILED` を見に行かない → 課金済みなのに失敗のまま、という注文を拾う仕組みがない。
- やり直しの途中で一度タイムアウトし、最後の試行が 503 で終わると、Unavailable（「課金されていない」）が投げられる。最後のエラーだけを見ると、課金済みかもしれないという情報が消える（retryingGateway.ts:35-44）。
- handler は 503 / payment_pending から 502 / payment_failed に変わった。外から見える API の形が変わっている（head/src/handlers/checkout.ts:24-32）。
- テストの期待値が PENDING_PAYMENT から PAYMENT_FAILED に書き換えられた。

**4. 8 つの分野**
1. 約束を守っているか: **兆しあり**。PENDING_PAYMENT の意味と照合バッチの役割が壊れている（base orderService.ts:56-57 → head :51-53）。
2. 失敗したらどうなるか: **兆しあり**。タイムアウトのあとの状態が「課金済み・注文は失敗」になりうる。さらに待ち時間は最大で約 30 秒（10 秒 × 3 回 + 待ち）になる（app.ts:12, retryingGateway.ts:24-25）。
3. 出したあとどうなるか: **兆しあり**。コードは戻せるが、課金済みで PAYMENT_FAILED になった注文と、24 時間を過ぎてからの二重課金は戻せない。API の応答の形も変わる。
4. 信頼の境界: **兆しなし**。新しい外からの入力はない。冪等キーはサーバ側で注文 ID から作っている。
5. 規模: **十分な場合に当たる**。1 件の注文につき最大 3 回呼ぶだけ。照合バッチは変わっていない。
6. 次に変える人: **兆し少し**。OrderService のコメントが「ゲートウェイでリトライ済み」という、型からは分からない前提に頼っている。`maxAttempts` / `baseDelayMs` は使う人がいない。
7. テスト: **兆しあり**。約束を守っていたテストが書き換えられた。「タイムアウト → 503」や「課金済みのタイムアウト」のテストがない。
8. AI のコード: **兆しあり**。頼まれた A（リトライ）に、状態の意味の変更 B が混ざっている。リトライの側は堅実に書けている。

**5. 選んだもの**
- 深く語る: 1・2・7。お金と、戻せない状態にかかわるため。
- 気にしなくてよいもの: 注入できる `sleep` / `random`（テストで使っている）、`findCharge` をリトライしないこと（バッチが 5 分ごとに再実行する）、規模。
- 良い点: 冪等キーの確認、同じキーでやり直すこと、カードの拒否はやり直さないこと、full jitter、決定的なテスト。
