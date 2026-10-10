## 変更前（base）が守っていた約束
- 状態の意味（base src/orders/order.ts:1、orderService.ts:37-40）: PENDING_PAYMENT は「課金したかどうか分からない」状態。checkout は CART か PAYMENT_FAILED のときだけ受け付ける（orderService.ts:33）。連打を防ぐ役目もある。
- PAYMENT_FAILED は「課金されていない。もう一度確定しても二重課金にならない」という意味（reconcilePendingPayments.ts:28 のコメント）。
- エラーの意味（gateway.ts:20-48）: Declined は何度送っても同じ。Unavailable は「課金されていない」。Timeout は「届いていて、課金済みかもしれない」。
- 失敗の扱い（base orderService.ts:51-58）: Declined だけを PAYMENT_FAILED にする。それ以外（Timeout、Unavailable、予期しない Error）は PENDING_PAYMENT のまま残し、照合バッチに任せる。
- 照合バッチ（reconcilePendingPayments.ts）: 5 分ごとに動き、10 分より古い PENDING_PAYMENT だけを見る。findCharge で課金の有無を確かめて PAID か PAYMENT_FAILED にする。PAYMENT_FAILED の注文は見ない。
- 冪等キー（order.ts:13-19、gateway.ts:6）: キーは注文 ID から作る。決済サービスが同じキーを 1 回分として扱うのは 24 時間だけ。
- 既存のテスト（base test/orderService.test.ts:37-41）が「タイムアウトなら PENDING_PAYMENT のまま」を守っていた。
- handler（base checkout.ts:24-31）: Unavailable と Timeout をまとめて 503 payment_pending にし、「注文履歴をご確認ください」と返していた。
- 慣習（README）: payment/ は外部との接続、orders/ は業務ロジック、app.ts は組み立て。

## 変更（head）との突き合わせ
- RetryingPaymentGateway（head retryingGateway.ts:30-54）: Unavailable と Timeout を最大 3 回まで送り直す。待ち時間は full jitter。冪等キーがなければ送らない。同じ req を使い回すので、キーは同じになる。決済サービスが 24 時間以内の同じキーを 1 回分として扱うなら（gateway.ts:6 のコメント）、送り直し自体は安全。→ 設計は妥当。
- app.ts で包んでいるだけ。server.ts の照合バッチも app.payments を使うが、findCharge はそのまま中身に渡しているだけ（retryingGateway.ts:47-49）。振る舞いは変わらない。
- **問題**: orderService の catch がすべてのエラーを PAYMENT_FAILED にした（head orderService.ts:50-56）。Timeout は「課金済みかもしれない」のに PAYMENT_FAILED になり、照合バッチ（PENDING_PAYMENT しか見ない）の対象から外れる。課金されたのに注文は未払いのまま残り、誰も直さない。
- さらに、PAYMENT_FAILED のままユーザーが 24 時間以上たってから確定し直すと、同じキーでも決済サービスは別の課金として扱う → 二重課金になりうる（キーの保持期間の外。推測）。
- 予期しない Error（httpGateway.ts:36 の想定外ステータス、res.json() の失敗など）も PAYMENT_FAILED になる。200 で課金されたのに本文の解析で落ちた場合も「失敗」になる（推測）。
- handler の文言「決済に失敗しました」も、Timeout では嘘になりうる。
- 既存のテストを書き換えている（orderService.test.ts:37-43）。偽の決済サービス（fakes.ts:15-21）は Error を投げるときに課金を記録しないので、「タイムアウトしたが実は課金されていた」場合を表せない。そのためテストは通る。
- import の削除（orderService.ts のハンク 0）は catch の変更に伴う機械的なもの。
- 待ち時間: タイムアウト 10 秒 × 3 回 ＋ 待ち最大 0.6 秒で、1 リクエストが約 30 秒かかりうる。server.ts にタイムアウトの設定はない。前段のプロキシなどの既定値はリポジトリから分からない。照合バッチの 10 分の猶予より十分短いので、処理中の注文にバッチが触ることはない。
- 型や制約で守れないか: エラーの型がすでに「課金されていない（Declined、Unavailable）」と「分からない（Timeout、その他）」を分けている。catch で分けるか、ゲートウェイが「確定した失敗」と「結果不明」を別の型で返すようにすれば、状態の意味を守れる。
- 修正案: リトライは残す。PAYMENT_FAILED にするのは Declined と Unavailable だけにする。Timeout と予期しないエラーは PENDING_PAYMENT のまま残し、handler で 503 pending に分ける。あるいは、その場で findCharge を呼んで確かめる。
