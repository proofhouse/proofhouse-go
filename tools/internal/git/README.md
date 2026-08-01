# `tools/internal/git`

Shell-out wrapper around the system git CLI. Returns Go-typed results for the read-only git operations that scanners, renderers, validators, and other repo-tooling want without each one re-writing the subprocess plumbing.

## Purpose

Tools under `tools/` regularly inspect the current worktree: branch, SHA, dirty state, ahead-behind counts against a base, the log, the diff. Each tool used to grow its own `exec.Command("git", ...)` paths. This package collects them in one place with typed results, consistent error handling, and one canonical answer per question.

The package doesn't embed [go-git](https://github.com/go-git/go-git). Native git runs faster and supports every git feature without library catch-up, while pulling in zero dependencies. Caching stays out of the package too. Caching policy belongs to the caller because the right policy varies by use case: per-render in contexttemplate, none at all in a one-shot scanner.

## API

```go
// Client wraps the git CLI. Construct one per logical operation. Method
// calls execute git as a subprocess. The client holds no state beyond the
// optional working directory override.
type Client struct{ /* ... */ }

// New returns a client rooted at the caller's working directory. Pass
// WithDir to override.
func New(opts ...Option) *Client

func WithDir(dir string) Option

func (c *Client) Branch(ctx context.Context) (string, error)
func (c *Client) SHA(ctx context.Context) (string, error)
func (c *Client) Dirty(ctx context.Context) (bool, error)
func (c *Client) AheadBehind(ctx context.Context, base string) (ahead, behind int, err error)
func (c *Client) Log(ctx context.Context, base string) ([]Commit, error)
func (c *Client) Diff(ctx context.Context, base string) (string, error)
func (c *Client) ToplevelPath(ctx context.Context) (string, error)
```

Each method returns the operation's natural Go type. Errors carry the subprocess's stderr so callers can surface git's own message rather than a wrapper string.

## Caching

Every call shells out. The package itself does no caching. Callers that re-ask for the same data within a logical unit of work hold their own memoization, typically a `sync.Once` or a small map keyed by method-plus-arguments.

Native git already caches its own object store on disk, so even repeated calls without caller-side memoization stay fast for read-only operations.

## Configuration

The package doesn't read configuration. The git CLI itself reads `~/.gitconfig`, `.git/config`, environment variables, and so on, exactly as it would on the command line. Tests inject behavior by pointing the client at a temporary directory via `WithDir`.

## Consumers

- `tools/contexttemplate`'s `worktree` registry composes `Branch`, `SHA`, `Dirty`, `AheadBehind`, `Log`, and `Diff` into the `Worktree` struct templates render against.
- `tools/validate-pr` (planned migration) absorbs its own current git shell-outs into this package.
- Future tooling under `tools/` that needs worktree state.

## Out of scope

- **Write operations.** Commit, push, pull, fetch, reset. Read-only suffices for the current consumers and avoids the risk of a scanner accidentally modifying state.
- **Porcelain abstractions.** This package returns what git returns. Higher-level concepts like "PR ready to merge" live in the consuming tool.
- **go-git embedding.** Calling out to the system git remains the right trade-off here.
- **Caching.** Caller responsibility.
