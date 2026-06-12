# replai - Go REPL Specification for AI Agents

## 1. Overview

`replai` is a REPL for AI coding agents (such as Claude Code) to debug and verify behavior in Go projects. It plays a role similar to Laravel's tinker, but the core difference is that it is optimized for **LLM read/write workflows**, not human interaction.

It allows immediate import of any package in the project and quick execution of functions, struct inspection, and return-value checks without writing test files.

### Design Principles

1. **Machine readability first** - All output must be structured and unambiguous to parse. No decorations (ANSI colors, box drawing, progress bars) should ever be printed.
2. **Determinism** - The same input should always produce the same output format. Output order and key order must be fixed.
3. **Token efficiency** - Do not waste LLM context. Do not emit verbose greetings, banners, or prompt markers. Large values must be abbreviated with explicit markers.
4. **Self-descriptiveness** - When errors or truncation occur, the output itself should indicate what to do next.
5. **Non-interactive mode as first-class** - AI agents rely on one-shot execution and stdin pipes more than interactive TTY sessions.

## 2. Execution Modes

### 2.1 One-Shot Mode (Primary Mode)

```sh
replai eval 'user.NewService(nil).Validate("foo")'
replai eval -f snippet.go        # read from file
echo '...' | replai eval -      # read from stdin
```

- Performs one code evaluation, prints the result as JSON to stdout, and exits.
- This is expected to be the most frequently used mode because AI agents invoke it through shell tools.

### 2.2 Session Mode

```sh
replai session start            # returns a session ID
replai session eval <id> '...'  # evaluates with preserved state (variables/imports)
replai session vars <id>        # lists defined variables
replai session end <id>
```

- Enables continuous debugging across variable definitions and imports.
- Session state must be persisted to files so it survives `eval` calls from separate processes (each AI shell invocation runs in an independent process).

### 2.3 Interactive Mode

```sh
replai repl
```

- Human fallback mode. Provided by specification, but not an optimization target.

## 3. Input Specification

### 3.1 Accepted Code

- Go expressions - their values are output targets
- Go statements - variable definitions such as `x := f()` are retained in the session
- Multi-line code blocks (including function/type definitions)
- Import declarations - writing `import "github.com/ensoria/rest"` must allow imports from any module in the monorepo and from external dependencies

### 3.2 Meta Commands

Operations other than code execution use meta commands prefixed with `:`.

| Command | Behavior |
|---|---|
| `:type <expr>` | Shows the type of an expression without evaluating it |
| `:doc <symbol>` | Shows doc comments for a package, function, or type |
| `:funcs <pkg>` | Lists signatures of exported functions and methods in a package |
| `:fields <type>` | Lists struct fields (including types and tags) |
| `:vars` | Lists defined variables and types in the session |
| `:imports` | Lists current imports |
| `:reset` | Resets session state |
| `:help` | Returns the list of meta commands in machine-readable JSON |

`:funcs` / `:fields` / `:doc` are core token-saving features that let AI inspect API shapes without reading source files directly.

## 4. Output Specification

### 4.1 Output Envelope

All output is written as **one JSON object** to stdout. No streaming is allowed (partial JSON is not parseable).

```json
{
  "ok": true,
  "value": {
    "repr": "&user.Service{DB: (*sql.DB)(nil), Timeout: 30000000000}",
    "type": "*github.com/ensoria/user.Service",
    "json": {"DB": null, "Timeout": 30000000000}
  },
  "stdout": "",
  "stderr": "",
  "defined": ["svc"],
  "duration_ms": 12,
  "truncated": false
}
```

| Field | Description |
|---|---|
| `ok` | Whether evaluation succeeded |
| `value.repr` | Go-like representation (equivalent to `%#v`); pointers are expanded to pointee values |
| `value.type` | Fully qualified type name |
| `value.json` | Included only when JSON-serializable; omitted if marshaling fails |
| `stdout` / `stderr` | Output produced by evaluated code (separate from evaluation result) |
| `defined` | Variable names newly defined or updated by this evaluation |
| `duration_ms` | Execution time |
| `truncated` | Whether output truncation occurred |

For multiple return values, `value` becomes an array named `values`. Return values of type `error` are separated into a dedicated `value.err` field and must explicitly indicate whether they are `nil`.

### 4.2 Error Output

Even on errors, with distinct exit codes, output must still use **the same envelope format on stdout** (do not print raw stack traces to stderr).

```json
{
  "ok": false,
  "error": {
    "kind": "compile",
    "message": "undefined: user.NewServce",
    "position": {"line": 1, "column": 8},
    "suggestion": "did you mean user.NewService? (similar symbols: NewService, NewServiceWithDB)"
  }
}
```

- `kind` must be one of `compile` | `runtime` | `panic` | `timeout` | `internal`.
- For `panic`, include stack trace in `error.stack`, but remove internal replai frames and keep only user-code frames.
- `suggestion` should include actionable fixes whenever possible (similar undefined symbols, missing import candidates, and so on). **The highest-priority requirement of this REPL is to return enough information for AI to decide the next step.**

#### Exit Codes

| Code | Meaning |
|---|---|
| 0 | Evaluation succeeded |
| 1 | Compile error / runtime error / panic (details in envelope) |
| 2 | Invalid replai usage (flag errors, etc.) |
| 124 | Timeout |

### 4.3 Value Display Rules

- **Pointers must display pointee values, not addresses**. Address strings like `0xc000010230` are meaningless for AI.
- Expand nesting up to depth 5 by default; beyond that, embed an omission marker such as `"...(depth limit, use --depth=N)"` including both reason and how to override.
- Slices/maps show up to the first 50 items by default. On omission, append a tail marker such as `"...(+120 items, use --max-items=N)"`.
- Long strings are cut at 2,000 characters by default and suffixed with `"...(+8500 chars)"`.
- `time.Time` should use RFC 3339; `time.Duration` should include both human-readable form like `"30s"` and nanoseconds.
- `[]byte` should be shown as string if valid UTF-8; otherwise show leading bytes in hex.
- Circular references should be terminated as `"<cycle: *user.Node>"`.
- Total output size limit is 16 KB by default (override with `--max-output`). When exceeded, set `truncated: true` and enumerate omitted parts in `truncated_fields`.

For all omissions, output must explicitly state both that omission occurred and how to retrieve the full result. Silent truncation is forbidden.

## 5. Project Integration

- Detect `go.mod` / `go.work` from the current directory and allow importing packages in that module (and workspace modules) with no extra configuration.
- Dependency packages must use versions specified in the project's `go.mod`.
- For references to packages not yet imported, attempt auto-import resolution equivalent to goimports. If resolved, report it in the `auto_imports` field.
- Internal packages must also be importable (for debugging use; relax module-boundary internal restrictions).

## 6. Safety and Resource Control

- Evaluations must have a default timeout of 30 seconds (overridable by `--timeout`) to avoid hanging an AI agent turn on infinite loops.
- Memory limits must be configurable (default 512 MB).
- File writes, network access, and `os/exec` are **allowed by default** (core for debugging tasks such as DB connection checks), but can be disabled with `--restrict`.
- Evaluated code and results must be recorded in a session log (JSONL), replayable via `replai session log <id>`, so AI can review what it tried earlier.

## 7. Non-Functional Requirements

- Target startup overhead for one-shot evaluation: under 1 second (using compile cache).
- JSON key order in output must be fixed to the order defined in this specification.
- All flags and subcommands must be discoverable from `replai --help`, and also available as JSON (`--help --json`), so AI can self-discover usage.

## 8. Non-Goals

- Human-oriented UX features such as syntax highlighting, completion, and history search
- Support for languages other than Go
- Remote execution or sandbox isolation (this is a local-development tool)
