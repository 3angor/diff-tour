import type { Line, Span } from './text.ts';

const KEYWORDS = new Set(
  (
    'abstract as async await break case catch class const constructor continue default delete do else enum ' +
    'export extends false finally for from function get if implements import in instanceof interface let new ' +
    'null of private protected public readonly return set static super switch this throw true try type typeof ' +
    'undefined var void while yield'
  ).split(' '),
);

export const STYLE = {
  keyword: '35',
  string: '32',
  number: '33',
  comment: '90',
  type: '36',
};

const TOKEN =
  /(\/\/.*)|(\/\*)|('(?:[^'\\]|\\.)*'?|"(?:[^"\\]|\\.)*"?|`(?:[^`\\]|\\.)*`?)|(\b\d[\d_]*(?:\.\d+)?\b)|([A-Za-z_$#][\w$]*)/y;

/** TypeScript / JavaScript をおおまかに色分けする。複数行のコメントだけは行をまたいで追う */
export function highlight(lines: string[]): Line[] {
  let inBlockComment = false;
  return lines.map((src) => {
    const spans: Span[] = [];
    const push = (text: string, style?: string) => {
      const last = spans.at(-1);
      if (last && last.style === style) last.text += text;
      else spans.push({ text, style });
    };
    let i = 0;
    while (i < src.length) {
      if (inBlockComment) {
        const end = src.indexOf('*/', i);
        if (end === -1) {
          push(src.slice(i), STYLE.comment);
          i = src.length;
        } else {
          push(src.slice(i, end + 2), STYLE.comment);
          i = end + 2;
          inBlockComment = false;
        }
        continue;
      }
      TOKEN.lastIndex = i;
      const m = TOKEN.exec(src);
      if (!m) {
        push(src[i]);
        i++;
        continue;
      }
      const [text, lineComment, blockStart, str, num, ident] = m;
      if (lineComment) push(text, STYLE.comment);
      else if (blockStart) {
        inBlockComment = true;
        push(text, STYLE.comment);
      } else if (str) push(text, STYLE.string);
      else if (num) push(text, STYLE.number);
      else if (ident) {
        if (KEYWORDS.has(ident)) push(text, STYLE.keyword);
        else if (/^[A-Z]/.test(ident)) push(text, STYLE.type);
        else push(text);
      }
      i += text.length;
    }
    return spans;
  });
}
