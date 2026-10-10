#!/usr/bin/env python3
"""第 2 回の生成の手順。

使い方: run2.py <branch> <C|Cs|B> <作業ディレクトリの親> <tag>

- 作業ディレクトリの親は prepare.sh で作ったもの（<親>/<branch>/ と <親>/clone/）。
  clone には main と feature/<branch> のローカルのブランチが要る（Go 版の再生のため）。
- 1 回ごとに <親>/runs/<branch>-<variant>-<tag>/ に入力を複製して、そこを cwd にする（並べて走らせても混ざらない）。
- 方針:
    C  … 1 回の呼び出しで、下調べを書いてからツアーの JSON を書く（区切りの行で取り出す）
    Cs … C と同じ。ただし --json-schema で {notes, tour} の構造化出力にする
    B  … 1 段目で下調べ、2 段目でツアー（第 2 版のプロンプト）
- 生成のあと validate.mjs と Go 版の --dump（⚠）で確かめ、誤りがあればエラーを添えて直させる（最大 2 回）。
- 結果は out2/<branch>-<variant>-<tag>.{json,notes.md,log.json} と、呼び出しごとの raw。
環境変数 TOUR_BIN があればそれを Go 版の再生に使う（なければ mise exec -- go run ./cmd/tour）。
"""
import json
import os
import re
import shutil
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
TOUR_REPO = os.path.abspath(os.path.join(HERE, '..', '..'))
# PROMPTS selects the prompt version (v2 by default; v3 is the catalog-based version)
PROMPTS = os.environ.get('PROMPTS', 'v2')
P = os.path.join(HERE, 'prompts', PROMPTS)
MAX_FIX = 2
SEP = '=== ツアー JSON ==='


def read(name):
    with open(os.path.join(P, name), encoding='utf-8') as f:
        return f.read().strip()


def claude_ro(work, prompt, schema=None):
    """読み取りの道具だけで claude -p を走らせる（前回の README と同じフラグ）。"""
    cmd = ['timeout', '1500', 'claude', '-p',
           '--restricted', '--strict-mcp-config',
           '--tools', 'Read,Grep,Glob',
           '--permission-mode', 'dontAsk', '--permission-prompts', 'none',
           '--no-session-persistence', '--output-format', 'json']
    if schema is not None:
        cmd += ['--json-schema', json.dumps(schema, ensure_ascii=False)]
    cmd.append(prompt)
    t0 = time.time()
    p = subprocess.run(cmd, cwd=work, capture_output=True, text=True)
    dt = time.time() - t0
    try:
        raw = json.loads(p.stdout)
    except json.JSONDecodeError:
        raw = {'is_error': True, 'result': '', 'stdout': p.stdout[-2000:], 'stderr': p.stderr[-2000:], 'returncode': p.returncode}
    raw['_wall_sec'] = dt
    return raw


def strip_json_text(text):
    text = text.strip()
    text = re.sub(r'^```(?:json)?\s*', '', text)
    text = re.sub(r'\s*```$', '', text)
    s, e = text.find('{'), text.rfind('}')
    return text[s:e + 1] if s >= 0 and e > s else text


def check(tour_text, clone, path):
    """JSON の文法、validate.mjs、Go 版の --dump の ⚠ を確かめ、誤りの一覧を返す。"""
    try:
        tour = json.loads(tour_text)
    except json.JSONDecodeError as ex:
        ctx = tour_text[max(0, ex.pos - 120):ex.pos + 120]
        return [f'JSON の文法の誤り: {ex.msg}（{ex.lineno} 行 {ex.colno} 桁）。前後: …{ctx}…'], None, None
    with open(path, 'w', encoding='utf-8') as f:
        json.dump(tour, f, ensure_ascii=False, indent=2)
    errors = []
    v = subprocess.run(['node', os.path.join(HERE, 'validate.mjs'), path, clone], capture_output=True, text=True)
    try:
        vr = json.loads(v.stdout)
        errors += [f'検証: {e}' for e in vr['errors']]
    except json.JSONDecodeError:
        vr = None
        errors.append(f'検証が動かなかった: {v.stderr[-500:]}')
    bin_ = os.environ.get('TOUR_BIN')
    cmd = ([bin_] if bin_ else ['mise', 'exec', '--', 'go', 'run', './cmd/tour']) + \
        [path, '--repo', clone, '--lang', 'ja', '--dump', '--plain', '--cols', '140', '--rows', '44']
    d = subprocess.run(cmd, cwd=TOUR_REPO, capture_output=True, text=True)
    warns = []
    step = '?'
    for line in d.stdout.splitlines():
        m = re.match(r'^===== step (\d+) =====', line)
        if m:
            step = m.group(1)
        for w in re.findall(r'⚠[^│]*', line):
            warns.append(f'再生（手順 {step}）: {w.strip()}')
    if d.returncode != 0:
        errors.append(f'再生が失敗した（終了コード {d.returncode}）: {d.stderr[-500:]}')
    errors += warns
    return errors, vr, d.stdout


def main():
    branch, variant, root, tag = sys.argv[1:5]
    assert variant in ('C', 'Cs', 'B')
    src = os.path.join(root, branch)
    clone = os.path.join(root, 'clone')
    name = f'{branch}-{variant}-{tag}'
    work = os.path.join(root, 'runs', name)
    out = os.path.join(HERE, os.environ.get('OUT', 'out2'))
    os.makedirs(out, exist_ok=True)
    if os.path.exists(work):
        shutil.rmtree(work)
    shutil.copytree(src, work)

    task = f'対象の変更: base は `main`、head は `feature/{branch}`。JSON の base と head にはこの名前をそのまま書く。'
    common, fmt = read('common.md'), read('format.md')
    tour_schema = json.load(open(os.path.join(HERE, 'tour.schema.json'), encoding='utf-8'))
    # CLI の --json-schema は $schema（2020-12 の宣言）を受け付けないので、渡すときは外す
    tour_schema = {k: v for k, v in tour_schema.items() if k not in ('$schema', '$id')}
    log = {'name': name, 'branch': branch, 'variant': variant, 'calls': [], 'rounds': []}

    def call(kind, prompt, schema=None):
        raw = claude_ro(work, prompt, schema)
        idx = len(log['calls'])
        with open(os.path.join(out, f'{name}.call{idx}-{kind}.raw.json'), 'w', encoding='utf-8') as f:
            json.dump(raw, f, ensure_ascii=False, indent=2)
        log['calls'].append({
            'kind': kind, 'cost': raw.get('total_cost_usd', 0), 'sec': round(raw['_wall_sec']),
            'turns': raw.get('num_turns'), 'is_error': raw.get('is_error'), 'stop_reason': raw.get('stop_reason'),
            'models': list((raw.get('modelUsage') or {}).keys()),
            'output_tokens': (raw.get('usage') or {}).get('output_tokens'),
            'denials': len(raw.get('permission_denials') or []),
        })
        return raw

    def generate(attempt):
        notes = ''
        if variant == 'B':
            r1 = call(f'stage1-{attempt}', read('stage1.md') + '\n\n' + read('stage1-output.md'))
            notes = r1.get('result', '')
            prompt = '\n\n'.join([common, task,
                                  '## 下調べの結果\nシニアエンジニアが事前に調べた「変更前のコードが守っていた約束」と、変更との突き合わせ。ツアーの核になる。内容は自分でもファイルを読んで確かめてから使う。\n\n' + notes,
                                  fmt, read('output-AB.md')])
            r = call(f'gen-{attempt}', prompt)
            text = strip_json_text(r.get('result', ''))
        elif variant == 'C':
            prompt = '\n\n'.join([common, task, read('planC.md'), fmt, read('output-C.md')])
            r = call(f'gen-{attempt}', prompt)
            res = r.get('result', '')
            if SEP in res:
                notes, body = res.split(SEP, 1)
                notes = notes.replace('=== 下調べ ===', '').strip()
            else:
                body = res
                log['separator_missing'] = True
            text = strip_json_text(body)
        else:  # Cs
            wrapper = {'type': 'object', 'additionalProperties': False, 'required': ['notes', 'tour'],
                       'properties': {'notes': {'type': 'string'}, 'tour': {'$ref': '#/$defs/tour'}},
                       '$defs': {**tour_schema['$defs'], 'tour': {k: v for k, v in tour_schema.items() if k not in ('$schema', '$id', '$defs')}}}
            prompt = '\n\n'.join([common, task, read('planC.md'), fmt, read('output-Cs.md')])
            r = call(f'gen-{attempt}', prompt, wrapper)
            so = r.get('structured_output') or {}
            notes = so.get('notes', '')
            text = json.dumps(so['tour'], ensure_ascii=False, indent=2) if 'tour' in so else r.get('result', '')
            log['structured_output_present'] = 'tour' in so
        return notes, text

    notes, text = generate(0)
    log['regenerations'] = 0
    if '{' not in text:
        # 返答に JSON が全くない（途中で止まった、など）。修正ではなく作り直しとして数える
        log['regenerations'] = 1
        notes, text = generate(1)

    with open(os.path.join(out, f'{name}.notes.md'), 'w', encoding='utf-8') as f:
        f.write(notes + '\n')

    final = os.path.join(out, f'{name}.json')
    for rnd in range(MAX_FIX + 1):
        errors, vr, dump = check(text, clone, final)
        log['rounds'].append({'round': rnd, 'errors': errors})
        if dump is not None:
            with open(os.path.join(out, f'{name}.dump{rnd}.txt'), 'w', encoding='utf-8') as f:
                f.write(dump)
        if vr is not None:
            log['validate'] = vr
        if not errors or rnd == MAX_FIX or '{' not in text:
            break
        with open(os.path.join(work, 'draft.txt'), 'w', encoding='utf-8') as f:
            f.write(text)
        fixp = '\n\n'.join([read('fix.md'), '## 検証の結果\n' + '\n'.join(f'- {e}' for e in errors), fmt, read('fix-output.md')])
        if variant == 'Cs':
            rf = call(f'fix{rnd + 1}', fixp.replace(read('fix-output.md'), '## 出力\n直したツアーの全体を構造化出力（StructuredOutput の道具）で返す。'), tour_schema)
            so = rf.get('structured_output')
            text = json.dumps(so, ensure_ascii=False, indent=2) if so else rf.get('result', '')
        else:
            rf = call(f'fix{rnd + 1}', fixp)
            text = strip_json_text(rf.get('result', ''))
    log['fixes'] = len(log['rounds']) - 1
    log['final_ok'] = not log['rounds'][-1]['errors']
    log['cost'] = round(sum(c['cost'] or 0 for c in log['calls']), 4)
    log['sec'] = sum(c['sec'] for c in log['calls'])
    with open(os.path.join(out, f'{name}.log.json'), 'w', encoding='utf-8') as f:
        json.dump(log, f, ensure_ascii=False, indent=2)
    with open(os.path.join(out, 'costs.jsonl'), 'a', encoding='utf-8') as f:
        f.write(json.dumps({'name': name, 'cost': log['cost'], 'sec': log['sec'], 'fixes': log['fixes'], 'final_ok': log['final_ok']}, ensure_ascii=False) + '\n')
    print(json.dumps({k: log[k] for k in ('name', 'cost', 'sec', 'fixes', 'final_ok')}, ensure_ascii=False))


if __name__ == '__main__':
    main()
