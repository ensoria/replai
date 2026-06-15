# replai — Go REPL for AI Agents

This document is written for LLM consumption. Every command prints **exactly one JSON object to stdout**. No decoration, banners, or prompt characters are ever emitted.

## When to use

- To verify the behavior of functions/types in a Go project instantly, without writing a test file
- To inspect the actual structure of a return value (nesting, types, values)
- To learn a package's API shape (function signatures, struct fields, docs) without reading source files (`:funcs` / `:fields` / `:doc` — saves tokens)

replai operates on the project found from the working directory (walks up to the nearest go.mod). **Run it inside the project directory.**

## Commands

| Command | Purpose |
|---|---|
| `replai eval '<code>'` | Evaluate code once (primary command; no state is kept) |
| `replai eval -f FILE` / `echo '<code>' \| replai eval -` | Evaluate from a file / stdin |
| `replai session start` | Start a session (variables and imports persist across processes) |
| `replai session eval <id> '<code>'` | Evaluate with the session state applied |
| `replai session vars <id>` | List session variables |
| `replai session log <id>` | Replay the evaluation history (recall what was tried before) |
| `replai session end <id>` | Delete a session |
| `replai --help --json` | Machine-readable self-description of all commands and flags |

## Accepted input

- Expression: `widget.New("x")` — the result goes into `value`
- Statements: `x := f()` — variables are kept in the session; redefining with `:=` is automatically rewritten to `=`
- Multiple statements + trailing expression: if the last line is an expression, its value becomes the result (REPL semantics)
- Declarations: `func` and `type` are hoisted to top level and usable from later snippets
- Import declarations: `import "github.com/ensoria/rest/pkg/rest"` — but **usually unnecessary** (see auto-import below)

### Auto-import

References to packages that are not imported are resolved automatically (goimports-equivalent) and reported in the `auto_imports` field. Importable: packages of the project itself, its `internal` packages, dependencies listed in go.mod, and sibling modules of the same monorepo (via an auto-generated go.work).

```sh
replai eval 'widget.New("demo")'   # widget resolves without an import
```

Auto-import succeeds only when the referenced symbol actually exists. A misspelled symbol produces `undefined: widget`; in that case check the correct name with `:funcs <import-path>` or add an explicit import.

## Output format (the envelope)

Success (actual output):

```json
{"ok":true,"value":{"repr":"&widget.Widget{ID: 42, Name: \"demo\", Tags: []string{\"a\", \"b\"}, Timeout: 30s (30000000000ns), CreatedAt: time.Time(\"2026-06-11T09:30:00Z\"), Parent: (*widget.Widget)(nil)}","type":"*example.com/fixture/pkg/widget.Widget","json":{"ID":42,"Name":"demo"}},"stdout":"","stderr":"","defined":[],"auto_imports":["example.com/fixture/pkg/widget"],"duration_ms":0,"truncated":false}
```

| Field | Meaning |
|---|---|
| `ok` | Whether evaluation succeeded |
| `value.repr` | Go-syntax-like rendering. Pointers are dereferenced (no addresses are printed) |
| `value.type` | Fully qualified type name (e.g. `*github.com/ensoria/rest/pkg/rest.Response`) |
| `value.json` | Present only when the value is JSON-marshalable |
| `value.err` | A trailing `error` return value: `{"nil":true}` or `{"nil":false,"message":"..."}` |
| `values` | Array used instead of `value` for multi-value returns |
| `stdout` / `stderr` | Output produced by the evaluated code (separated from the result) |
| `defined` | Variable names defined or updated by this snippet |
| `auto_imports` | Import paths resolved automatically |
| `duration_ms` | Execution time of the snippet itself |
| `truncated` / `truncated_fields` | Whether output was trimmed by `--max-output` (default 16KB), and which fields |

A function returning a non-nil `error` still yields **`ok: true`** (an error value is result data, not an evaluation failure). Always check `value.err`:

```json
{"ok":true,"value":{"repr":"0","type":"int","json":0,"err":{"nil":false,"message":"widget exploded","type":"*errors.errorString"}},"stdout":"","stderr":"","defined":[],"auto_imports":["example.com/fixture/pkg/widget"],"duration_ms":0,"truncated":false}
```

## Errors and exit codes

| exit | `error.kind` | Meaning | What to do |
|---|---|---|---|
| 0 | — | Success | Check whether `value.err` is non-nil |
| 1 | `compile` | Compile error | Follow `error.position` (line/column within your snippet) and `error.suggestion` |
| 1 | `panic` | Panic occurred | Read `error.stack` (already cleaned and remapped to `snippet:<line>` form) |
| 1 | `runtime` | Process died abnormally (os.Exit, goroutine panic, etc.) | Read `error.message` / `error.stack` |
| 2 | `internal` | replai misuse (run outside a project, bad flag, --restrict violation) | Follow the message |
| 124 | `timeout` | Killed after `--timeout` (default 30s) | Partial stdout/stderr is preserved; suspect blocking calls |

Errors use the same envelope format on stdout. `suggestion` tells you the next step (actual output):

```json
{"ok":false,"error":{"kind":"compile","message":"undefined: widget.Nwe","position":{"line":1,"column":8},"suggestion":"did you mean widget.New? (similar symbols: New); use `:funcs example.com/fixture/pkg/widget` to list the package API"},"stdout":"","stderr":"","defined":[],"duration_ms":0,"truncated":false}
```

## Meta commands (inspect without executing)

Pass as the argument of `eval` / `session eval`, starting with `:`. **Try these before reading source files** (saves tokens).

| Command | Example |
|---|---|
| `:funcs <pkg>` | `replai eval ':funcs github.com/ensoria/rest/pkg/rest'` — exported function/method signatures with first doc line |
| `:fields <pkg>.<Type>` | `replai eval ':fields github.com/ensoria/rest/pkg/rest.Response'` — field names, types, tags |
| `:doc <pkg>[.<Symbol>]` | `replai eval ':doc github.com/ensoria/rest/pkg/rest.NewRequest'` |
| `:type <expr>` | `replai session eval <id> ':type w.Rename("z")'` — static type without evaluating |
| `:vars` / `:imports` | List session variables / imports |
| `:reset` | Clear the session |
| `:help` | List meta commands (JSON) |

`<pkg>` is a full import path or a relative one (`./pkg/rest`).

## Critical session constraint: re-execution semantics

Each session eval **re-executes every prior entry** before running the new snippet (only the new snippet's output is shown). Consequences:

- Side effects of past entries (DB writes, HTTP calls, file creation) are **repeated on every eval**
- For sequences of side-effecting operations, putting multiple statements into a single `eval` is safer than a session
- If a past entry no longer compiles (e.g. a type was redefined), `error.message` says `in session entry N`; recover with `:reset`

## Flags (shared by all commands)

| Flag | Default | Meaning |
|---|---|---|
| `--timeout` | `30s` | Evaluation timeout (exit 124 when exceeded) |
| `--depth` | `5` | Nesting depth expanded in repr; deeper parts become `...(depth limit, use --depth=N)` |
| `--max-items` | `50` | Slice/map items shown; overflow becomes `...(+N items, use --max-items=N)` |
| `--max-str` | `2000` | String length shown; overflow becomes `...(+N chars, ...)` |
| `--max-output` | `16384` | Byte cap for the whole envelope |
| `--max-mem` | `512MiB` | GOMEMLIMIT for the child process (best effort) |
| `--restrict` | off | Statically reject imports of `os`, `os/exec`, `net`, `syscall` |

Every omission or truncation is explicitly marked. Silent truncation never happens.

## Limitations and caveats

- **Side effects repeat due to re-execution** (above). This is replai's biggest caveat
- When the last element of a multi-value result is an untyped nil, it is assumed to be a nil `error` (heuristic based on Go's `(T, error)` convention)
- Nested internal packages like `pkg/x/internal/y` cannot be imported (module-root `internal/` works)
- `--restrict` is a static import check only; it does not detect network access performed inside imported packages
- The first eval may take a few seconds while project dependencies compile; subsequent evals run in under a second thanks to the build cache
- Evaluated code can write files, use the network, and spawn subprocesses by default (this is a debugging tool); effects may reach outside the generated `.replai/` directory (which is self-gitignored)
- Session files live in `<project>/.replai/sessions/`; concurrent evals are serialized with flock

## Typical workflow

```sh
# 1. Inspect the API shape (no execution)
replai eval ':funcs github.com/ensoria/rest/pkg/rest'
replai eval ':fields github.com/ensoria/rest/pkg/rest.Response'

# 2. One-shot behavior check
replai eval 'rest.NewFileResponse(200, "/tmp/a.txt", "a.txt", "text/plain", false)'

# 3. Use a session for continuous debugging
replai session start                       # => {"ok":true,"session_id":"s1a2b3c4",...}
replai session eval s1a2b3c4 'svc := rest.NewRequest(nil)'
replai session eval s1a2b3c4 ':type svc'
replai session eval s1a2b3c4 'svc.Method'
replai session end s1a2b3c4
```

## Internals (only when debugging replai itself)

Evaluated code is generated into `<project>/.replai/run/main.go` and built/run with the real compiler (`go build`). Compile error positions are already remapped to snippet coordinates, so you normally never need to look at this file. Deleting `.replai/` is safe; it is regenerated on the next eval.
