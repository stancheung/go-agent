# Architecture

`go-agent` is a single-binary, terminal-based coding agent. It talks to any
OpenAI-compatible `/chat/completions` endpoint and runs a tool-calling loop so
the model can read, write, and edit files and execute shell commands.

Design bias: **small and stdlib-first.** One external dependency (`godotenv`),
no framework, no DI container, no code generation. Every layer is readable in
one sitting.

---

## 1. Request lifecycle

```
                    ┌──────────────────────────────────────────────┐
                    │                 cmd/main.go                  │
                    │        build ctx → cli.NewRepl().Run()       │
                    └───────────────────────┬──────────────────────┘
                                            │
                    ┌───────────────────────▼──────────────────────┐
                    │              internal/cli                    │
                    │  config.NewConfig() → ai.NewProvider()       │
                    │  agent.New() → RegisterTool × 4 (+skills)    │
                    │  → skills.Load("skills") → REPL loop         │
                    └───────────────────────┬──────────────────────┘
                                            │  Run(ctx, input)
                    ┌───────────────────────▼──────────────────────┐
                    │              internal/agent                  │
                    │  History + tool registry + tool loop         │
                    └───────┬──────────────────────────┬───────────┘
                            │                          │ Execute(ctx, argsJSON)
                    ┌───────▼────────┐        ┌────────▼───────────┐
                    │  internal/ai   │        │  extensions/tools  │
                    │ HTTP transport │        │ 4 concrete tools   │
                    └───────┬────────┘        └────────┬───────────┘
                            │ POST {base}/chat/completions
                     ┌──────▼─────────────────────────▼──────────┐
                     │        OpenAI-compatible LLM server        │
                     └────────────────────────────────────────────┘
```

A single turn:

1. `Repl` reads a line from stdin and calls `Agent.Run`.
2. `Run` appends the user message to `History`.
3. `nextReply` builds an `ai.Request` (`Agent.SystemPrompt` as a leading
   `system` message + full `History` + sorted tool defs + `tool_choice:
   "auto"`) and POSTs it via `Provider.ChatCompletion`.
4. If `Choices[0].Message.ToolCalls` is non-empty, all calls are executed
   **concurrently** (`sync.WaitGroup` + `wg.Go`), each output appended as a
   `role: "tool"` message keyed by `tool_call_id` (results are collected in
   call order before appending); go to 3. `Run` also prints each tool call to
   stdout (`Tool: name args`) — the REPL does not print anything about tools.
5. Otherwise the trimmed text content is returned to the REPL and printed.

---

## 2. Package map

| Package | Responsibility | Imports (internal) |
|---|---|---|
| `cmd` | Process entry point; builds a cancellable `context.Context`. | `internal/cli` |
| `internal/cli` | REPL: stdin scanning, provider/agent construction, tool registration, error printing. | `config`, `extensions/tools`, `internal/agent`, `internal/ai` |
| `internal/agent` | Conversation state and the tool loop. Owns `History`. | `extensions/tools`, `internal/ai` |
| `internal/ai` | Wire types (`provider.go`) + HTTP transport (`client.go`). | — (stdlib only) |
| `extensions/tools` | `Tool` interface + `read_file`, `write_file`, `edit_file`, `bash`. | — (stdlib only) |
| `extensions/skills` | `SKILL.md` discovery, prompt catalog, and the `load_skill` tool. | `extensions/tools` (schema helpers) |
| `config` | Loads `.env` into `Config{BaseURL, Model, APIKey}`. | `github.com/joho/godotenv` |

### Dependency rules

```
        cmd
         │
    internal/cli ──────────────┬──────────────┐
         │                     │              │
   internal/agent ──────► extensions/tools    │
         │                                     │
    internal/ai ◄─────────────────────────────┘
         │
      config
```

Three rules hold today and are load-bearing:

- **`internal/ai` depends on nothing but stdlib.** It is the lowest layer and
  knows nothing about tools, agents, or the terminal. This is what lets
  `client_test.go` exercise it against a plain `httptest.Server`.
- **`extensions/tools` and `extensions/skills` must never import
  `internal/*`.** They live outside `internal/` specifically so
  `internal/agent` is allowed to import them. The dependency arrow points
  `agent → tools`, never the reverse. Tools are plain values with no agent
  knowledge (`extensions/skills` may import `extensions/tools` for the schema
  helpers; `load_skill` satisfies `tools.Tool` structurally).
- **`config/` is outside `internal/`** so external consumers can import it.
  `internal/*` is compiler-enforced module-private; `config/` and
  `extensions/` are not.

`extensions/tools` is the only package with a *contract* rather than just a
location constraint: implement the four-method `Tool` interface and the agent
will accept it.

---

## 3. Layer details

### `internal/ai` — types vs. transport split

`provider.go` is **types only**: the OpenAI chat/tool/embedding wire schema,
with pointer fields + `omitempty` so unset options are absent from the payload
rather than sent as zero values. No methods, no logic.

`client.go` holds all behavior:

- `NewProvider(ctx, baseURL, model, apiKey)` — `ctx` is accepted but unused;
  `Provider` is a plain `{BaseURL, Model, APIKey}` struct.
- `ChatCompletion(ctx, req)` — non-streaming POST. It deliberately takes the
  **full `Request`** rather than a message slice, so callers control tools,
  `ToolChoice`, temperature, etc. `req.Model` defaults to the provider's model.
- URL is `TrimRight(baseURL, "/") + "/chat/completions"` — appended verbatim, so
  `baseUrl` **must** include the API-version prefix (`http://host:8731/v1`).
- `Authorization: Bearer` is set **only when `apiKey != ""`**, which is what makes
  an unauthenticated local llama.cpp endpoint work.
- Package-level `httpClient` with a 2-minute `Timeout` bounds any single call so
  a hung endpoint cannot wedge the REPL.

Unused-but-intentional: `StreamChunk`, `StreamChoice`, and the embedding types
are defined for planned streaming/embedding support. `Request.Stream` exists but
is never set by the agent.

### `internal/agent` — the loop

```go
type Agent struct {
    Provider     *ai.Provider
    History      []ai.Message
    Tools        map[string]tools.Tool
    SystemPrompt string // leading system message; never part of History
}
```

`New` fills `SystemPrompt` via `defaultSystemPrompt()`: OS, cwd, the probed
shell description, shell guidelines, and a file rule (task-generated files go
under `tmp/`, never the project root) — so the model knows it is on PowerShell
before it tries POSIX syntax. `internal/cli` later appends the skills catalog
to it.

`Run` is the heart of the system. Its contract:

- **Bounded.** `maxToolIterations = 8` caps LLM round-trips per `Run`; hitting
  the cap is an error, not a silent truncation.
- **Transactional.** `start := len(a.History)` is captured up front; every
  failure path calls `rollback(start, err)`, truncating `History` back to its
  pre-`Run` length. A failed turn leaves the conversation exactly as it was, so
  the user can just retry.
- **Two error classes.** Transport failures, empty replies, no-choices replies,
  and loop-cap exhaustion **fail the `Run`**. Unknown tool names, unparseable
  argument JSON, and tool execution errors are **returned as text**
  (`"error: ..."` in a `tool` message) so the model can self-correct. This
  distinction is deliberate: a typo'd path is a model mistake, not a system
  failure.
- **Deterministic payloads.** `toolDefs()` sorts tool names before building
  definitions, so the request body is stable across runs regardless of Go map
  iteration order. Tool calls inside one reply run concurrently, but results
  are written to a pre-sized slice by index and appended in call order.
- Assistant replies are appended with `Role` forced to `"assistant"`, and each
  tool result copies `call.ID` into a local before taking its address, so the
  `*string` `ToolCallID` fields don't all alias one loop variable.

`New(ctx, provider)` accepts a `context.Context` it does not store or use —
retained as a construction-time hook.

### `extensions/tools`

`Tool` is four methods: `Name`, `Description`, `Parameters`, and
`Execute(ctx, argsJSON) (string, error)`. `Parameters()` returns an arbitrary
JSON-Schema value; helpers `Schema`, `StrParam`, and `IntParam` build the
object shape. Every tool unmarshals `argsJSON` into an anonymous struct and
validates required fields itself, returning a parse/validation error that the
agent converts into model-visible text.

Shared limits live in `tool.go`:

| Constant | Value | Applies to |
|---|---|---|
| `maxReadLines` | 2000 | `read_file` line cap |
| `maxReadBytes` | 51200 | `read_file` byte cap |
| `maxToolOutput` | 32768 | `bash` output truncation |
| `defaultBashTimeout` | 120 (s) | `bash` when no `timeout` given |

Behavioral notes worth knowing:

- `read_file` prefixes `N: ` line numbers, clamps `offset` to ≥ 1 and `limit` to
  ≤ 2000, uses a 1 MiB `bufio.Scanner` buffer for long lines, and refuses
  directories explicitly.
- `edit_file` enforces **exactly one** match — zero matches and multiple matches
  are both errors with instructions ("read the file first", "include more
  surrounding context") written to steer the model into recovering.
- `write_file` `MkdirAll`s the parent directory (0755) and writes 0644.
- `bash` merges stdout and stderr into one buffer, picks `powershell.exe
  -Command` vs `bash -c` from `runtime.GOOS`, and reports non-zero exit codes and
  timeouts **as output text with a nil error** — a failing command is
  information for the model, not an agent failure.

### `extensions/skills` — progressive disclosure for instructions

A skill is a directory `skills/<name>/SKILL.md` with `---` frontmatter (a
minimal key:value parser — no YAML dep). Three moving parts:

- `skills.Load(dir)` scans for skill dirs at startup and returns
  `[]Skill{Name, Description, Path}` sorted by name. Missing `skills/` is not
  an error; empty `SKILL.md` files are skipped. The list is a **startup
  snapshot** — adding a skill needs a restart, but bodies are re-read from
  disk on every `load_skill` call.
- `skills.Catalog()` renders a one-line-per-skill index that `internal/cli`
  appends to `Agent.SystemPrompt`, with the rule "call `load_skill` when a
  task matches". Only the catalog is always in context; bodies are not.
- `LoadSkillTool` (`load_skill`) returns the requested skill's body with
  frontmatter stripped, **prefixed with a `Skill directory: <abs path>`
  header** warning that the shell cwd is not the skill dir. Without that
  header the model runs the skills' relative `scripts/...` paths from the
  repo root and fails — this was the single biggest real-world breakage.

`skills/` in this repo holds third-party packs (`pdf`, `pptx`, `xlsx`,
`doc-processing`, `searxng-search`) that shell out to Python; see AGENTS.md
for their runtime deps.

### `config`

`NewConfig()` calls `godotenv.Load()` and `log.Fatalf`s if `.env` is absent.
`Load` (not `Overload`) means real environment variables take precedence over
`.env` values. Because `godotenv` resolves relative to the **current working
directory**, the binary must be run from the repo root — that is a hard failure,
not a fallback.

Env var names are non-standard camelCase: `baseUrl`, `model`, `apiKey`
(`apiKey` optional).

---

## 4. Known gaps

These are open edges, not bugs to route around silently:

- **No context-window management.** `History` grows monotonically for the
  process lifetime — no trimming, summarization, or token accounting. Long
  sessions will eventually exceed the model's window.
- **No streaming.** `StreamChunk`/`StreamChoice` types exist; the client is
  POST-and-decode only.
- **`Plugin` is aspirational.** `internal/agent/plugin.go` declares an interface
  requiring only `Name()`; nothing implements or consumes it, and there is no
  plugin loader.
- **`extensions/mcp` is an empty directory.** MCP support is planned, not
  implemented.
- **Skill catalog is a startup snapshot.** New `skills/` directories need a
  process restart to appear (skill body edits apply immediately).
- **No sandbox or path allowlist.** `bash` and `write_file` operate with the
  agent process's full privileges on any reachable path.

---

## 5. Testing strategy

Tests are hermetic — no live server, no network, no shared fixtures.

| Package | Approach |
|---|---|
| `internal/ai` | `httptest` fake `/chat/completions`; asserts request shape, auth-header presence/absence, and HTTP-error surfacing. |
| `internal/agent` | `newTestAgent(t, handler)` drives the loop against a scripted fake endpoint: history carry-over, rollback-on-failure, tool execution, error feedback, loop cap, empty reply, no choices. |
| `extensions/tools` | `runTool` + `t.TempDir()` for real filesystem round-trips of read/write/edit, plus `bash`. |
| `extensions/skills` | `t.TempDir()` skill trees: frontmatter parsing, dir-name fallback, `load_skill` round-trip incl. the `Skill directory:` header, unknown-name errors. |
| `cmd`, `config`, `internal/cli` | No tests — thin wiring and process-level I/O. |

Verify with `go build ./...` → `go vet ./...` → `go test ./...`, and `gofmt -w .`
before committing (no CI or linter exists to catch formatting).

---

## 6. How to extend

**Add a tool.** Create a file in `extensions/tools`, implement the four `Tool`
methods, reuse `Schema`/`StrParam`/`IntParam`, respect the shared output caps,
then register it in `internal/cli/repl.go`. Do not import `internal/*` from
here.

**Add a skill.** Drop `skills/<name>/SKILL.md` with `name`/`description`
frontmatter (restart to register it). If it ships scripts, reference them
relative to the skill dir — `load_skill`'s `Skill directory:` header tells the
model where they actually live.

**Swap the model/endpoint.** Change `.env` only — `baseUrl` (with version prefix),
`model`, optional `apiKey`. No code change.

**Add streaming.** Add a method beside `ChatCompletion` in `internal/ai` using
the existing `StreamChunk` types; `agent.Run` is the only caller to migrate.

**Add context compaction.** `Agent.History` is the single source of truth and
`rollback` already demonstrates index-based truncation — a compaction step
belongs in `Run` between iterations, not in `internal/ai`.
