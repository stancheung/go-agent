# AGENTS.md

## Status

Basic agent loop works: `cmd/main.go` → `internal/cli` (stdin REPL) → `internal/agent` → `internal/ai`, and `go build ./...` / `go vet ./...` / `go test ./...` all pass.

## Commands

Run from the repo root (see env note below).

- Run the app: `go run ./cmd`
- Rebuild the binary: `go build -o go-agent.exe ./cmd` — **`go build ./...` does NOT refresh `go-agent.exe`**; running the exe after code changes silently runs stale code (this has bitten real debugging sessions).
- Verify: `go build ./...` → `go vet ./...` → `go test ./...`
- Focused tests: `go test ./internal/agent` or `go test ./extensions/skills -run TestName`
- Format: `gofmt -w .` — run before committing; the repo has no CI or linter to catch it.

Only `gofmt` is installed. Tests live only in `internal/agent`, `extensions/skills`, and `extensions/tools` (scripted fake `/chat/completions` endpoint and temp-dir file/skill tests — no real server needed). `internal/ai`, `internal/cli`, `config`, and `cmd` have no tests.

## Environment

- `.env` is loaded at startup by `config.NewConfig()` via `godotenv.Load()`, which reads `.env` from the **current working directory** and `log.Fatalf`s if absent. Running the binary from anywhere but the repo root is a hard failure.
- Env var names are non-standard camelCase: `baseUrl`, `model`, `apiKey` (`apiKey` optional — Authorization header only sent when non-empty; the local `.env` omits it). The local `.env` actually uses uppercase `BASE_URL`/`MODEL` — works only because Windows env vars are case-insensitive; `config.go` reads `os.Getenv("baseUrl")`, which would break on Linux.
- `baseUrl` must include the API-version prefix: the client appends `/chat/completions` verbatim. The working value is `http://fedora:8731/v1` (a live llama.cpp-style local OpenAI-compatible endpoint); the bare `http://fedora:8731` yields 404/405.
- Skill-specific env vars (e.g. `SEARXNG_URL` for the searxng-search skill) also go in `.env` — `godotenv.Load` injects them into the process env and `bash` tool subprocesses inherit them.
- **CJK + PowerShell pipes:** Python printing non-ASCII text into a PowerShell pipe raises `UnicodeEncodeError` (stdout encodes as cp1252) — and `2>$null` hides the traceback, making it look like empty output. Set `$env:PYTHONIOENCODING='utf-8'` first, or write UTF-8 files from inside Python (`io.open(..., encoding='utf-8')`) and read them back. This cost a whole debugging session; `skills/pdf/SKILL.md` has a Windows/CJK section.

## Architecture

- `internal/ai/provider.go` is **types only** — the OpenAI chat/tool/embedding wire schema. `internal/ai/client.go` holds all transport: `ChatCompletion(ctx, Request)` is a **non-streaming POST** (takes the full request so tools incl. `ToolChoice` ride along; `req.Model` defaults to the provider model); `NewProvider(ctx, baseURL, model, apiKey)`. The `StreamChunk`/`StreamChoice` types exist but nothing streams yet.
- `internal/agent/agent.go` keeps `Agent.History []ai.Message` and a `Tools map[string]tools.Tool` registry (`RegisterTool`). `Run(ctx, input) (string, error)` runs a tool loop: append user turn → LLM → if the reply has `tool_calls`, execute them **concurrently** (`sync.WaitGroup` + `wg.Go`, results collected in call order before appending), then repeat — capped at 8 LLM round-trips. Tool execution/arg errors are fed back to the model as text, not failed. `Run` itself prints each tool call to stdout (`Tool: name args`) — the REPL does not. Plain text replies return; any failure rolls history back to the start of the `Run`. `Agent.SystemPrompt` (built by `defaultSystemPrompt`: OS, cwd, probed shell + version, shell rules, and a file rule — generated files go under `tmp/`, never the project root) is prepended to every request as a `system` message but kept out of `History`.
- `extensions/tools` (outside `internal/`, so `internal/agent` imports it but it may not import `internal/*`) holds the `Tool` interface and four tools: `read_file`, `write_file`, `edit_file`, `bash`. `bash` picks the shell by `runtime.GOOS` (`powershell.exe -NoProfile -NonInteractive -Command` on Windows, `bash -c` elsewhere), prefixes results with an `exit: N` line, and on Windows rejects POSIX-isms (`&&`, `||`, `/dev/null`) before executing with an actionable error. `tools.ShellDescription()` probes the PowerShell version once; `tools.ShellGuidelines()` supplies the shell do's/don'ts reused by the agent's system prompt. Unknown tools, empty/`tool_calls`-only replies, and the tool-loop cap all surface as errors from `Run`.
- `extensions/skills` discovers `<cwd>/skills/<name>/SKILL.md` files at startup (`skills.Load`), renders a name+description catalog that `internal/cli/repl.go` appends to `Agent.SystemPrompt`, and provides the `load_skill` tool (registered only when skills exist) that returns a skill's body with frontmatter stripped. Skill list is a startup snapshot — new files need a restart; **skill bodies are re-read from disk on every `load_skill` call** (body edits need no restart). Frontmatter is a minimal `---` key:value parser, no YAML dep. `load_skill` output is prefixed with a `Skill directory: <abs path>` header warning that the shell cwd is NOT the skill dir — without it the model runs the skills' relative `scripts/...` paths from the repo root and fails.
- `skills/` holds third-party skill packs (`pdf`, `pptx`, `xlsx`, `doc-processing`, `searxng-search`). They shell out to Python — the `pdf` skill's `pdf_read.py` needs `pypdf` + `pdfplumber` (installed via pip); `pymupdf`/`marker-pdf` paths are optional and NOT installed. Skill docs are bash-centric; on Windows the model must translate (`$env:VAR`, `curl.exe`) — `searxng-search` and `pdf` SKILL.md files carry Windows sections for this.
- `config/` sits outside `internal/`, so it is importable by external consumers; `internal/*` is not.
- `extensions/mcp` is an empty placeholder directory (MCP support is planned, not implemented). The `Plugin` interface (`internal/agent/plugin.go`) only requires `Name()`; nothing implements or consumes it.
- `ARCHITECTURE.md` is a detailed walkthrough of the same design; keep it in sync when changing the loop, transport, or package boundaries.

## Conventions

- Single module (`go-agent`), Go 1.27.0, one dependency (`godotenv`). Prefer stdlib; check `go.mod` before adding anything.
- New tools: implement the four `Tool` methods in `extensions/tools` (reuse `Schema`/`StrParam`/`IntParam`), register in `internal/cli/repl.go`; never import `internal/*` from there.
- Local git repo on branch `main` (initial commit covers the working tree as of 2026-11). `.gitignore` excludes `.env`, `*.exe`, `tmp/`, `*Zone.Identifier`, `__pycache__/`. `core.autocrlf=true` is in effect: files are LF on disk, stored as LF.
