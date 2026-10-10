import { DatabaseSync } from 'node:sqlite';
import type { Repo } from './git.ts';

// マイグレーションをメモリ上の SQLite に当てて、ある時点のスキーマを取り出す。
// リポジトリの中身は読むだけで、ファイルには何も書かない

export type Column = {
  name: string;
  type: string;
  /** 型より後ろの定義（NOT NULL・DEFAULT・CHECK・REFERENCES など） */
  constraints: string;
};

export type Index = { name: string; columns: string[]; unique: boolean };

export type Table = { name: string; columns: Column[]; indexes: Index[] };

export type Schema = Map<string, Table>;

// メモリ上の DB の外にファイルを作れてしまう文は実行しない
const UNSAFE = /\b(ATTACH|VACUUM|load_extension)\b/i;

export function loadSchema(repo: Repo, ref: string, dir: string): Schema | string {
  const files = repo.listFiles(ref, dir).filter((f) => f.endsWith('.sql')).sort();
  const db = new DatabaseSync(':memory:');
  try {
    for (const file of files) {
      const sql = repo.show(ref, file)?.join('\n') ?? '';
      if (UNSAFE.test(sql)) return `安全のため実行しません: ${file}`;
      try {
        db.exec(sql);
      } catch (e) {
        return `マイグレーションを当てられません: ${file}（${(e as Error).message}）`;
      }
    }
    const schema: Schema = new Map();
    const tables = db
      .prepare("SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
      .all() as Array<{ name: string; sql: string }>;
    for (const t of tables) {
      const defs = columnDefs(t.sql);
      const columns = (db.prepare(`PRAGMA table_info(${quote(t.name)})`).all() as Array<{ name: string; type: string }>).map(
        (c) => ({
          name: c.name,
          type: c.type,
          constraints: (defs.get(c.name) ?? '').replace(new RegExp(`^${escape(c.type)}\\s*`, 'i'), ''),
        }),
      );
      const indexes = (
        db.prepare(`PRAGMA index_list(${quote(t.name)})`).all() as Array<{ name: string; unique: number; origin: string }>
      )
        // 主キーのために自動で作られる索引は、列の定義を見ればわかるので省く
        .filter((i) => i.origin !== 'pk')
        .map((i) => ({
          name: i.name,
          unique: i.unique === 1,
          columns: (db.prepare(`PRAGMA index_info(${quote(i.name)})`).all() as Array<{ name: string }>).map((c) => c.name),
        }));
      schema.set(t.name, { name: t.name, columns, indexes });
    }
    return schema;
  } finally {
    db.close();
  }
}

function quote(name: string): string {
  return `"${name.replace(/"/g, '""')}"`;
}

function escape(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/** CREATE TABLE 文を、列ごとの定義に分ける（コメントを除き、括弧の中のカンマでは区切らない） */
function columnDefs(sql: string): Map<string, string> {
  const body = sql
    .slice(sql.indexOf('(') + 1, sql.lastIndexOf(')'))
    .replace(/--[^\n]*/g, '');
  const parts: string[] = [];
  let depth = 0;
  let quoteChar = '';
  let cur = '';
  for (const ch of body) {
    if (quoteChar) {
      if (ch === quoteChar) quoteChar = '';
    } else if (ch === "'" || ch === '"') quoteChar = ch;
    else if (ch === '(') depth++;
    else if (ch === ')') depth--;
    else if (ch === ',' && depth === 0) {
      parts.push(cur);
      cur = '';
      continue;
    }
    cur += ch;
  }
  parts.push(cur);
  const defs = new Map<string, string>();
  for (const part of parts) {
    const text = part.replace(/\s+/g, ' ').trim();
    const m = text.match(/^["`[]?(\w+)["`\]]?\s*(.*)$/);
    if (m) defs.set(m[1], m[2]);
  }
  return defs;
}

export type Change = 'added' | 'removed' | 'changed' | 'same';

export type TableDiff = {
  name: string;
  change: Change;
  columns: Array<Column & { change: Change }>;
  indexes: Array<Index & { change: Change }>;
};

export function diffTable(before: Table | undefined, after: Table | undefined): TableDiff {
  const name = (after ?? before)!.name;
  const change: Change = !before ? 'added' : !after ? 'removed' : 'same';
  const columns: TableDiff['columns'] = [];
  for (const c of after?.columns ?? []) {
    const old = before?.columns.find((o) => o.name === c.name);
    columns.push({
      ...c,
      change: !before ? 'same' : !old ? 'added' : old.type !== c.type || old.constraints !== c.constraints ? 'changed' : 'same',
    });
  }
  for (const c of before?.columns ?? []) {
    if (after && !after.columns.some((n) => n.name === c.name)) columns.push({ ...c, change: 'removed' });
  }
  const indexes: TableDiff['indexes'] = [];
  for (const i of after?.indexes ?? []) {
    indexes.push({ ...i, change: before && !before.indexes.some((o) => o.name === i.name) ? 'added' : 'same' });
  }
  for (const i of before?.indexes ?? []) {
    if (after && !after.indexes.some((n) => n.name === i.name)) indexes.push({ ...i, change: 'removed' });
  }
  const touched = columns.some((c) => c.change !== 'same') || indexes.some((i) => i.change !== 'same');
  return { name, change: change === 'same' && touched ? 'changed' : change, columns, indexes };
}
