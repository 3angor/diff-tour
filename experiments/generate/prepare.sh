#!/usr/bin/env bash
# 使い方: prepare.sh <題材リポジトリ> <作業ディレクトリの親> <branch>...
# 題材を複製し、ブランチごとに base/（分岐点）と head/ を .git なしのディレクトリとして書き出す。
# base は main の先端ではなく、分岐点（merge-base）にする。差分（main...head）と中身を揃えるため。
set -euo pipefail
src=$1 root=$2; shift 2
mkdir -p "$root"
[[ -d $root/clone ]] || git clone -q "$src" "$root/clone"
for branch in "$@"; do
  d=$root/$branch
  rm -rf "$d"; mkdir -p "$d/base" "$d/head"
  mb=$(git -C "$root/clone" merge-base origin/main "origin/feature/$branch")
  git -C "$root/clone" archive "$mb" | tar -x -C "$d/base"
  git -C "$root/clone" archive "origin/feature/$branch" | tar -x -C "$d/head"
  git -C "$root/clone" diff "origin/main...origin/feature/$branch" > "$d/change.diff"
  git -C "$root/clone" log --format='%h %s%n%n%b' "origin/main..origin/feature/$branch" > "$d/commits.txt"
done
