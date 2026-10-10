#!/usr/bin/env bash
# Records demo/demo-en.gif and demo/demo-ja.gif with vhs.
# Set EXAMPLE_REPO to use a local copy of diff-tour-example instead of cloning it.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
example=${EXAMPLE_REPO:-https://github.com/init-co-jp/diff-tour-example}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

go build -o "$work/bin/tour" "$root/cmd/tour"
# Recording must never call an LLM: if a cached tour is missed, this stand-in fails at once
printf '#!/bin/sh\necho "demo: claude must not run" >&2\nexit 1\n' > "$work/bin/claude"
chmod +x "$work/bin/claude"

git clone -q "$example" "$work/repo"
for b in $(git -C "$work/repo" for-each-ref --format='%(refname:strip=3)' refs/remotes/origin); do
  [ "$b" = HEAD ] || git -C "$work/repo" branch -q -f "$b" "origin/$b" 2>/dev/null || true
done
git -C "$work/repo" checkout -q feature/coupons

# The cache key must match generate.CacheKey: merge base, head, language, prompt version, backend.
prompt_version=$(sed -n 's/^const PromptVersion = "\(.*\)"/\1/p' "$root/internal/generate/generate.go")
mb=$(git -C "$work/repo" merge-base main feature/coupons)
head=$(git -C "$work/repo" rev-parse feature/coupons)

for lang in en ja; do
  home="$work/$lang"
  mkdir -p "$home/config/tour" "$home/cache/tour/tours"
  printf '{"lang":"%s","llm":"claude-code"}\n' "$lang" > "$home/config/tour/config.json"
  key=$(printf '%s\0%s\0%s\0%s\0%s\0' "$mb" "$head" "$lang" "$prompt_version" "Claude Code" | sha256sum | cut -c1-24)
  cp "$root/tours/example-coupons.$lang.json" "$home/cache/tour/tours/$key.json"
  DEMO_BIN="$work/bin" DEMO_REPO="$work/repo" DEMO_HOME="$home" vhs "$root/demo/demo-$lang.tape"
done
