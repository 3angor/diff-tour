#!/usr/bin/env bash
# 使い方: run.sh <payment-retry|coupons> <A|B> <作業ディレクトリの親>
# 作業ディレクトリ（<親>/<branch>）は prepare.sh で作っておく（head/ base/ change.diff commits.txt）。
set -euo pipefail
branch=$1 variant=$2 root=$3
here=$(cd "$(dirname "$0")" && pwd)
work=$root/$branch
out=$here/out
mkdir -p "$out"

# 読み取りの道具（Read・Grep・Glob）だけを許し、作業ディレクトリの外は読めないようにする
claude_ro() {
  (cd "$work" && timeout 1500 claude -p \
    --restricted --strict-mcp-config \
    --tools "Read,Grep,Glob" \
    --permission-mode dontAsk --permission-prompts none \
    --no-session-persistence --output-format json "$1")
}

task="対象の変更: base は \`main\`、head は \`feature/$branch\`。JSON の base と head にはこの名前をそのまま書く。"

if [[ $variant == A ]]; then
  prompt="$(cat "$here/prompts/common.md")

$task

$(cat "$here/prompts/format.md")

$(cat "$here/prompts/output.md")"
  claude_ro "$prompt" > "$out/$branch-A.raw.json"
else
  claude_ro "$(cat "$here/prompts/stage1.md")" > "$out/$branch-B1.raw.json"
  notes=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['result'])" "$out/$branch-B1.raw.json")
  printf '%s\n' "$notes" > "$out/$branch-B1.notes.md"
  prompt="$(cat "$here/prompts/common.md")

$task

## 下調べの結果
シニアエンジニアが事前に調べた「変更前のコードが守っていた約束」と、変更との突き合わせ。ツアーの核になる。内容は自分でもファイルを読んで確かめてから使う。

$notes

$(cat "$here/prompts/format.md")

$(cat "$here/prompts/output.md")"
  claude_ro "$prompt" > "$out/$branch-B.raw.json"
fi

# 結果の本文から JSON を取り出す（囲みが付いていた場合も外す）
python3 - "$out/$branch-$variant.raw.json" "$out/$branch-$variant.json" <<'PY'
import json, re, sys
raw = json.load(open(sys.argv[1]))
text = raw.get('result', '').strip()
text = re.sub(r'^```(?:json)?\s*|\s*```$', '', text)
start, end = text.find('{'), text.rfind('}')
tour = json.loads(text[start:end + 1])
json.dump(tour, open(sys.argv[2], 'w'), ensure_ascii=False, indent=2)
print('ok', sys.argv[2])
PY
