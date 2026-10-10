// 生成したツアー JSON を検証する。
// 使い方: node validate.mjs <tour.json> <題材リポジトリ（複製）>
// 形、指しているファイルと行の実在、差分のハンク番号の実在を確かめ、数を数える。読むだけで何も書かない。
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';

const [file, repo] = process.argv.slice(2);
const tour = JSON.parse(readFileSync(file, 'utf8'));
const errors = [];
const warnings = [];

function git(args) {
  try {
    return execFileSync('git', ['-C', repo, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
  } catch {
    return null;
  }
}

// 複製ではブランチが origin/ の下にある
function ref(name) {
  return git(['rev-parse', '--verify', '-q', name]) !== null ? name : `origin/${name}`;
}

const base = ref(tour.base);
const head = ref(tour.head);
const changed = new Set((git(['diff', '--name-only', `${base}...${head}`]) ?? '').split('\n').filter(Boolean));
const fileCache = new Map();
function lines(at, path) {
  const key = `${at}:${path}`;
  if (!fileCache.has(key)) {
    const out = git(['show', key]);
    fileCache.set(key, out === null ? null : out.replace(/\n$/, '').split('\n'));
  }
  return fileCache.get(key);
}
function hunkCount(path) {
  return ((git(['diff', '-U3', `${base}...${head}`, '--', path]) ?? '').match(/^@@/gm) ?? []).length;
}
function width(s) {
  let w = 0;
  for (const ch of s) w += /[ᄀ-ᅟ⺀-꓏가-힣豈-﫿︰-﹏＀-｠￠-￦]/.test(ch) ? 2 : 1;
  return w;
}

const KINDS = ['p', 'quote', 'list', 'pre', 'table', 'code', 'diff', 'snippet', 'schema', 'rows', 'target'];
const stats = { chapters: 0, steps: 0, questions: 0, basis: { fact: 0, inferred: 0, opinion: 0, none: 0 }, kinds: {}, codeRefs: 0, unchangedFiles: new Set(), files: new Set() };

if (tour.version !== 1) errors.push('version が 1 ではない');
if (!tour.title || !tour.base || !tour.head || !Array.isArray(tour.chapters)) errors.push('title / base / head / chapters がない');
const steps = (tour.chapters ?? []).flatMap((c, ci) => {
  stats.chapters++;
  if (!c.title || !Array.isArray(c.steps)) errors.push(`第${ci + 1}章: title / steps がない`);
  return (c.steps ?? []).map((s) => ({ ...s, ci }));
});

steps.forEach((step, si) => {
  const at = `手順${si + 1}「${step.title}」`;
  stats.steps++;
  if (!step.title || !Array.isArray(step.blocks)) errors.push(`${at}: title / blocks がない`);
  if (step.question) {
    stats.questions++;
    if (si === steps.length - 1) errors.push(`${at}: 問いの手順が最後にある（答えの手順がない）`);
    else if (steps[si + 1].question) warnings.push(`${at}: 問いの次の手順も問い`);
  }
  for (const block of step.blocks ?? []) {
    const keys = Object.keys(block).filter((k) => KINDS.includes(k));
    if (keys.length !== 1) {
      errors.push(`${at}: ブロックの種類が判断できない ${JSON.stringify(block).slice(0, 80)}`);
      continue;
    }
    const kind = keys[0];
    stats.kinds[kind] = (stats.kinds[kind] ?? 0) + 1;
    if (kind === 'p') {
      const b = block.basis ?? 'none';
      if (!(b in stats.basis)) errors.push(`${at}: basis が不正 ${b}`);
      else stats.basis[b]++;
    }
    if (kind === 'pre') {
      for (const l of block.pre.split('\n')) if (width(l) > 76) warnings.push(`${at}: 図の行が 76 桁を超える（${width(l)} 桁）`);
    }
    if (kind === 'code') {
      const c = block.code;
      stats.codeRefs++;
      stats.files.add(c.file);
      if (!changed.has(c.file)) stats.unchangedFiles.add(c.file);
      const src = lines(c.at === 'base' ? base : head, c.file);
      if (!src) {
        errors.push(`${at}: ファイルがない ${c.file}（${c.at ?? 'head'}）`);
        continue;
      }
      const [a, b] = c.lines ?? [];
      if (!(a >= 1 && b >= a && b <= src.length)) errors.push(`${at}: lines が範囲外 ${c.file} ${a}-${b}（全 ${src.length} 行）`);
      if (c.show) {
        const [s0, s1] = c.show;
        if (!(s0 >= 1 && s1 >= s0 && s1 <= src.length)) errors.push(`${at}: show が範囲外 ${c.file} ${s0}-${s1}`);
        else if (a < s0 || b > s1) warnings.push(`${at}: 強調する行が表示範囲の外 ${c.file}`);
      }
    }
    if (kind === 'diff') {
      const d = block.diff;
      stats.files.add(d.file);
      const n = hunkCount(d.file);
      if (n === 0) errors.push(`${at}: 差分がない ${d.file}`);
      for (const h of d.hunks ?? []) if (!(h >= 0 && h < n)) errors.push(`${at}: ハンク ${h} がない ${d.file}（${n} 個）`);
    }
    if (kind === 'schema') {
      if (!(git(['ls-tree', '--name-only', head, '--', block.schema.migrations]) ?? '').trim()) errors.push(`${at}: マイグレーションのディレクトリがない ${block.schema.migrations}`);
    }
    if (kind === 'rows') {
      const r = block.rows;
      if (r.data.some((row) => row.length !== r.head.length)) errors.push(`${at}: rows の列数が head と合わない`);
    }
  }
});

console.log(
  JSON.stringify(
    {
      file,
      ok: errors.length === 0,
      errors,
      warnings,
      stats: { ...stats, unchangedFiles: [...stats.unchangedFiles], files: [...stats.files] },
    },
    null,
    2,
  ),
);
