# tour

**English** | [日本語](README.ja.md)

> Status: early prototype. It plays back hand-written tours. Generating tours with an LLM is not implemented yet.

AI can write code that works, but it does not teach you *why* the code is written that way.
`tour` walks you through a change the way a senior engineer would read it: what the change does to the behavior of the whole system, which existing code it depends on (even files that did not change), why a seemingly redundant line is there, and where a senior would stop and say "wait".

You only need two keys: **→** next and **←** back.

## Try it

Requires Go 1.27 and git.

```sh
go run ./cmd/tour tours/payment-retry.json --repo path/to/tour-sample-shop
```

On the first run, `tour` asks two things: the language (detected from your terminal, English by default) and the LLM that will write tours. Run `tour setup` to change them later.

| Key | Action |
| --- | --- |
| `→` `space` `enter` | Next step |
| `←` `backspace` | Previous step |
| `o` | Open the code of this step in your editor (`$VISUAL` / `$EDITOR`, or `TOUR_EDITOR="code -g {file}:{line}"`) |
| `q` | Quit |
| `↑` `↓` | Scroll, only when a step does not fit on the screen |

## What a step can show

- Code and diffs, always read from git at the right commit, with syntax highlighting for any language
- Code that is **not** in the repository (a naive alternative, a suggested fix), always labeled as such
- Flow and timeline diagrams
- Database schemas before and after the change, computed by applying the migrations to an in-memory SQLite database (SQLite migrations only for now)
- Tables of example data
- Questions that make you think before the answer is shown on the next step

Every explanation is labeled with how it is known: **Verified** (backed by code, tests or commit messages), **From the code** (inferred from the structure), or **Senior view** (general design judgment, not verified in this repository).

## Read-only by design

`tour` never writes to your working tree or `.git`. When you open a file that differs from the commit being toured, it opens a read-only copy under your cache directory instead.

## Development

```sh
go test ./...
go run ./cmd/tour tours/coupons.json --repo path/to/repo --dump --cols 140 --rows 44 --plain
```

`--dump` prints every step at a fixed screen size, which is handy for checking layouts without a terminal.
