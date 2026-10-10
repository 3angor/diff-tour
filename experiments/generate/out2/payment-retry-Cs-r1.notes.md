## 変更前（base）の約束
- 状態の意味（base order.ts:1、orderService.ts:37-38）: PENDING_PAYMENT は「課金されたか分からない／処理中」、PAYMENT_FAILED は「課金されていないと分かっている。もう一度確定してよい」。checkout は CART か PAYMENT_FAILED のときだけ受け付ける（orderService.ts:33）。
- エラーの意味（base gateway.ts:20-48）: Declined は何度やっても同じ結果。Unavailable は「課金はされていない」。Timeout は「課金済みの可能性がある」。
- 失敗の扱い（base orderService.ts:51-58）: Declined のときだけ PAYMENT_FAILED にする。ほかのエラーは PENDING_PAYMENT のまま残して照合バッチに任せる。
- 照合バッチ（base reconcilePendingPayments.ts:17-31、server.ts:23-25）: 5 分ごとに、10 分以上前の PENDING_PAYMENT だけを findCharge で確かめる。PAYMENT_FAILED の注文は見ない。
- 冪等キー（order.ts:13-19、gateway.ts:6）: order-<id>。決済サービスが保持するのは 24 時間。
- 既存のテスト（base test/orderService.test.ts:37-41）が「タイムアウトなら PENDING_PAYMENT のまま」を守っていた。
- 連打対策: 先に PENDING_PAYMENT を保存して状態チェックで弾いている（アプリのコードの手順で守っている。DB の制約ではない）。

## 変更との突き合わせ
- RetryingPaymentGateway（head retryingGateway.ts）: Unavailable/Timeout のとき最大 3 回やり直し、待ち時間は full jitter。同じ req を送るので冪等キーも同じ。リトライ自体は冪等キーのおかげで安全。
- 問題: head orderService.ts:50-55 で、どんなエラーでも PAYMENT_FAILED にするようになった。Timeout（課金済みかもしれない）でも「課金されていない」状態になる。バッチは PAYMENT_FAILED を見ないので、課金済みの注文が失敗のまま残る。24 時間たってからやり直すと二重課金になりうる。
- 想定外のエラー（httpGateway.ts:35-36、retryingGateway.ts:32）も PAYMENT_FAILED になる。
- 既存のテストが書き換えられた（head test:37-43）。fakes.ts:15-21 はエラーを投げる手順では課金を記録しないので、「課金されたうえでタイムアウト」を表せない。
- ハンドラ（head checkout.ts:24-31）は Timeout でも「失敗、もう一度」と返して、やり直しを促している。
- 応答の時間: 10 秒 × 3 回 + 待ち時間で最悪およそ 30 秒（inferred）。
- 問題なしと確かめたもの: 連打対策は残っている。findCharge はそのまま中身に渡している。旧コードが残した PENDING の注文はバッチが拾う。app.ts・import の削除は機械的な変更。
- 型で守れる案: 冪等キーを branded type にすれば実行時チェックが不要になる。「結果不明」を状態やエラーの型で表す。
