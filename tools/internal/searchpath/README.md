# `tools/internal/searchpath`

PATH-style name and path resolution over a list of `fs.FS` sources. Takes an ordered list of labeled sources, resolves names or relative paths against them with first-match-wins semantics, and returns a `Match` that names the source that won and the valid `fs.FS` path inside it.

## Purpose

Project tooling that loads files by logical name (template partials, rules documents, configuration fragments) wants the same lookup model that Unix `$PATH` gives binaries. That model gives an ordered list of sources with first-match-wins behavior, plus a layering API for prepending or appending. Without a shared package, each consumer grows its own search loop with slightly different rules for case sensitivity, escape prevention, and missing-entry behavior.

This package centralizes that lookup. Both lookup modes run over one source list. A name-with-extension mode handles partial-style references like `{{ template "worktree" . }}`, and a verbatim relative-path mode handles path-style references like `{{ readFile "commit-workflow.md" }}`.

A source pairs an `fs.FS` with a display label. Disk directories wrap through `os.DirFS(absDir)`. Bundled assets wrap an `embed.FS` directly, often through `fs.Sub` to strip an internal prefix. Any custom `fs.FS` works too. The resolver treats every source the same way.

## API

```go
// Source is a labeled fs.FS. Label appears in error messages and debug
// output. The FS is whatever the caller supplies.
type Source struct {
    Label string
    FS    fs.FS
}

// Match describes a successful lookup. Path is a valid fs.FS path
// (forward slashes, no leading slash, no `..` segments) inside Source.FS.
// Callers read the bytes via fs.ReadFile(m.Source.FS, m.Path).
type Match struct {
    Source Source
    Path   string
}

// Resolver searches sources in order. Construct one per logical
// operation; the resolver itself holds no state beyond the source list.
type Resolver struct{ /* ... */ }

// New returns a resolver over the given sources.
func New(sources []Source) *Resolver

// DiskSources wraps each disk directory in os.DirFS and uses the
// directory string as the label. Convenience for callers that already
// carry a []string of directories (CLI flag parsing, env-var lists).
func DiskSources(dirs []string) []Source

// Sources returns the configured sources in lookup order. Useful for
// logging the active path or rendering it in a debug appendix.
func (r *Resolver) Sources() []Source

// Name resolves a logical name by appending `ext` and trying both
// `<name>.<ext>` and `_<name>.<ext>` in each source in order. First
// match wins. The underscore variant matches Helm's partial-file
// convention.
func (r *Resolver) Name(ctx context.Context, name, ext string) (Match, error)

// Path resolves a relative path verbatim against each source. The path
// is interpreted as a valid fs.FS path inside each source. First match
// wins.
func (r *Resolver) Path(ctx context.Context, relPath string) (Match, error)
```

A missing match returns a `*NotFoundError` carrying the requested name or path plus the configured sources walked. Callers that want to render the search path in a debug appendix or surface it in an error message read the sources from there.

## Escape prevention

`fs.FS` semantics rule out paths that escape their source. Both `embed.FS.Open` and `os.DirFS(...).Open` reject paths that fail `fs.ValidPath`: absolute paths, leading slashes, `..` segments, and backslashes. The resolver inherits that protection without adding its own checks.

The resolver rejects absolute paths. A consumer that needs to read files outside the source list reads them directly via `os.ReadFile`.

## Caching

The package does no caching. Filesystem stat calls already run cheaply, and the OS handles its own caching for disk-backed sources. Embed-backed sources hold their data in the binary, so a read costs nothing more than a memory access. A consumer that resolves the same name many times in one logical operation can wrap the resolver with its own memoization.

## Configuration

Construction-time options carry every knob the resolver exposes. The package doesn't read environment variables itself. Consumers that build a path from env vars (like `$CLAUDE_SKILL_DIR` plus `$CLAUDE_PROJECT_DIR`) assemble the slice themselves and pass it to `New`.

## Consumers

- `tools/contexttemplate`'s loader uses `Name` to resolve partial references during the AST walk.
- `tools/contexttemplate`'s `include` and `readFile` template funcs use `Name` and `Path` respectively.
- `tools/contexttemplate`'s bundled partial set sits as a `Source` at the tail of the default search path, wrapping the package's `embed.FS` (via `fs.Sub` to strip the internal `partials/` prefix).
- `tools/validate-pr` (planned migration) can use `Path` for its relative-link checks, wrapping the repo root through `DiskSources`.

## Out of scope

- **Glob expansion.** The resolver matches exact names and exact relative paths. Glob walking falls to the consumer.
- **Symlink policy beyond OS defaults.** The package follows symlinks the same way `os.DirFS` does. Strict no-follow semantics need a consumer-side wrapper.
- **Absolute paths.** A consumer that needs to read by absolute path uses `os.ReadFile` directly, since the resolver only ever walks its configured sources.
- **Cross-process source sharing.** Each consumer constructs its own resolver per logical operation rather than reaching for a shared singleton.
- **Caching.** Source-level caching covers current callers.
