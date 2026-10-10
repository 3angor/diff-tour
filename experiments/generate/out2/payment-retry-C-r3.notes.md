下調べは終わりました。いまは、決済のタイムアウト後に注文を PAYMENT_FAILED にする変更が、照合バッチの前提を崩している点を軸にツアーを組み立てています。


- **変更前の約束（base）**
  - 注文の状態には決まった意味がある。`PENDING_PAYMENT` は「課金されたか分からない」。`PAYMENT_FAILED` は「課金されていないと分かっている。もう一度確定しても二重課金にならない」（base `src/jobs/reconcilePendingPayments.ts:28`、`src/orders/orderService.ts:56-57`）。
  - 失敗したときの扱い：カードの拒否だけを `PAYMENT_FAILED` にしている。障害やタイムアウトでは `PENDING_PAYMENT` のまま残し、照合バッチに任せている（base `orderService.ts:51-59`）。
  - 照合バッチは、10 分以上たった `PENDING_PAYMENT` **だけ**を `findCharge` で確かめ、`PAID` か `PAYMENT_FAILED` にする（`reconcilePendingPayments.ts:5-6,17-31`）。5 分ごとに動き、`app.payments` を使う（`server.ts:23-25`）。
  - エラーの意味：`PaymentUnavailableError` は「課金はされていない」、`PaymentTimeoutError` は「課金が済んでいる可能性がある」（`gateway.ts:31,42`）。冪等キーは決済サービス側で 24 時間保持される（`gateway.ts:6`）。キーは `paymentKeyFor` で作る（`order.ts:13-19`）。
  - 既存のテストが守っていた振る舞い：「タイムアウトしたら PENDING_PAYMENT のまま残す（課金されたか分からないため）」（base `test/orderService.test.ts:37-41`）。
  - これらの約束は、すべてアプリのコードの手順とコメントで守られていて、型や制約では守られていない。
- **変更（head）**
  - `RetryingPaymentGateway` を追加した。一時的な障害なら最大 3 回まで、待ち時間を指数的に伸ばしてばらつかせ（full jitter）、同じ冪等キーでやり直す。`findCharge` はそのまま中身に渡す。
  - `app.ts` で、この新しいクラスに包んで組み込んでいる。
  - `OrderService` は、**どんなエラーでも** `PAYMENT_FAILED` にするように変わった。
  - 画面に返す応答は 503 `payment_pending` から 502 `payment_failed`「決済に失敗しました」に変わった。
  - 既存のテストは、タイムアウトなら `PAYMENT_FAILED` になる、という期待に書き換えられた。
- **観点ごとに確かめた結果**
  - 既存の約束：**破っている**。タイムアウトを使い切ると、課金済みかもしれない注文が `PAYMENT_FAILED` になる。照合バッチは `PENDING_PAYMENT` しか見ないので、この注文は二度と照合されない。ユーザーが 24 時間たってから確定し直すと、冪等キーの保持期間が切れていて二重課金になりうる（推測）。
  - 最後のエラーの種類が、それまでの試行の結果をすべて表すとは限らない。1 回目がタイムアウトで最後が 503、ということがある（推測）。
  - 予期しないエラーも `PAYMENT_FAILED` になる。2xx の応答を受けたあと JSON の読み取りで落ちた場合が含まれる（`httpGateway.ts:35-39`、推測）。
  - 接続や設定：組み込んでいる場所は `createApp` 1 か所だけ。照合バッチの `findCharge` はやり直さずにそのまま渡る。`timeoutMs` は 10 秒なので、最悪およそ 30.6 秒かかる（推測）。`OrderService` のテストはやり直しを通らない。
  - 書かれた慣習：`#` で始まる非公開フィールド、`Options` 型と `??` による既定値、日本語の doc コメント、時間や乱数を外から渡せる作り（`InMemoryOrderRepository(now)` と同じ）。いずれも既存のコードに従っている。
  - 型や制約：「課金されていないと分かっている」と「分からない」の区別を、エラーの型や戻り値の型で表す設計を、選ばなかった案として比べられる。
  - 同時実行：やり直しの間も `PENDING_PAYMENT` なので、ボタンの連打は 409 で弾かれる。照合バッチとも 10 分の猶予があるのでぶつからない。新旧のコードが共存する間も、古い `PENDING_PAYMENT` はバッチが拾う。
  - テスト：`ScriptedPaymentGateway` は、エラーを返すときに課金を記録しない（`fakes.ts:18-19`）。そのため「時間切れだったが課金は済んでいた」を表せず、書き換えたテストは作りの上で必ず通る。
  - 仕分け：import の削除は機械的（`PaymentDeclinedError` を使う場所がなくなっただけ）。`app.ts` は小さいが、新しい仕組みを有効にする場所。時間をかけるべきなのは、`orderService` の catch、`checkout` の応答、書き換えられたテストの 3 か所。
