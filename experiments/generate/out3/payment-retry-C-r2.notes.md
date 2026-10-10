下調べが終わったので、タイムアウト時の状態遷移を中心にツアーを組み立てる。


- **最初の仮説**: 決済サービスが一時的に 503 を返したとき、注文確定の中で自動でやり直す。それでも駄目なら注文を PAYMENT_FAILED にして、ユーザーがその場でやり直せるようにする。困らなくなるのは「注文履歴を見に行け」と言われていたユーザー（commits.txt:3-5）。
- **変更前のコードが守っていた約束（base）**
  - `PENDING_PAYMENT` は「課金されたか分からない」という意味。障害やタイムアウトのときはこの状態のまま残し、照合バッチに判断を任せていた（base/src/orders/orderService.ts:51-58）。
  - 照合バッチが拾うのは、`PENDING_PAYMENT` のまま 10 分たった注文だけ（base/src/jobs/reconcilePendingPayments.ts:17-20）。課金の記録がなければ PAYMENT_FAILED にする（同 :27-29）。
  - `PaymentUnavailableError` の意味は「課金はされていない」、`PaymentTimeoutError` の意味は「課金が済んでいる可能性がある」（base/src/payment/gateway.ts:31,42）。
  - 冪等キーは決済サービス側で 24 時間保持される（gateway.ts:6）。キーは `paymentKeyFor` で作ることが決まっている（order.ts:13-19）。
  - 連打は「先に PENDING_PAYMENT を保存してから状態を確かめる」という手順で防いでいる（orderService.ts:37-40）。DB の制約で守っているわけではなく、アプリの手順で守っている。
  - 既存のテストが「タイムアウトなら PENDING_PAYMENT のまま」を守っていた（base/test/orderService.test.ts:37-41）。
  - 偽の決済サービスは、エラーを投げたときに課金を記録しない（test/fakes.ts:15-20）。そのため「タイムアウト＝課金なし」という前提で動いている。
- **突き合わせ**
  - RetryingPaymentGateway は約束を守っている。同じキーでやり直すので、タイムアウトをやり直しても二重課金にならない（head/src/payment/retryingGateway.ts:30-45）。
  - orderService は、例外の種類を問わず PAYMENT_FAILED にするようになった（head/src/orders/orderService.ts:50-55）。Unavailable では正しいが、Timeout では「課金済みかもしれない」注文を「失敗」と言い切ってしまい、照合バッチの対象から外れる。
  - handler は Timeout のときも「決済に失敗しました」と返す（head/src/handlers/checkout.ts:24-31）。
- **8 分野**
  1. 約束: 兆しあり。PENDING_PAYMENT の意味と Timeout の意味を壊している（gateway.ts:42、base orderService.ts:56-57）。
  2. 失敗: 兆しあり。3 回とも Timeout だと、課金済みで PAYMENT_FAILED になりうる。照合バッチも拾わない。
  3. 出したあと: 兆しあり。お金に関わり、戻せない。API の応答の形も 503/payment_pending から 502/payment_failed に変わっている（checkout.ts の差分）。スキーマの変更はない（インメモリ）。
  4. 信頼の境界: 兆しなし。新しい入力の経路はない。
  5. 規模: 兆しは弱い。最悪で 10 秒 × 3 回と待ち時間で 30 秒強、要求を待たせる。照合バッチの 10 分よりは十分短い（reconcile:6）。やり直しの層はこれ 1 つだけ。
  6. 次に変える人: 小さな兆し。orderService のコメント「リトライ済みなので」は app.ts の組み立てに頼っている。RetryingPaymentGateway 自体は、包むだけで分離が良い。
  7. テスト: 兆しあり。既存テストの期待値を書き換えている。偽物の Timeout は課金を記録しないので、本物（gateway.ts:42）と違う振る舞いをする。
  8. AI: 兆しあり。テストを実装に合わせて書き換えている。冪等キーの確認は防御だが、安く、守る相手が実在するので見合っている。使っている関数はすべて実在する。
- **深く語る分野**: 1・2・7。お金に関わり、戻せないうえに、テストが通っているので見逃されやすい。
- **気にしなくてよいもの**: タイムアウトのやり直し自体による二重課金（冪等キーがある）。やり直しが二重になること（ほかの層にやり直しはない）。照合バッチとの競合（30 秒強に対してバッチは 10 分）。
- **良いところ**: やり直しをゲートウェイの包みに閉じた点。full jitter。sleep と random を差し替えられる点。Declined はやり直さない点。冪等キーがない呼び出しを拒む点。
