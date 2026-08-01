# `tools/internal/repofile`

Reads files from paths relative to a repo root. The package abstracts the question of locating the repo root, then exposes a small reader interface for consumers that need to inline file content into their output.

## Purpose

Project tooling under `tools/` regularly reads small text files from the repo. The list includes rules documents, configuration snippets, template fragments, and assorted other inputs. Each consumer used to grow its own `git rev-parse --show-toplevel` plus `os.ReadFile` pair. This package centralizes the pattern so one set of decisions about root resolution, error wrapping, and `..` escape prevention applies everywhere.

## API

```go
// Reader reads files relative to Root. Construct one per consumer that
// wants to read project files. Methods take a context.Context for
// cancellation, even though file I/O itself doesn't honor it; the
// pattern matches the other tools/internal packages.
type Reader struct {
    Root string
}

// New returns a reader rooted at the result of `git rev-parse
// --show-toplevel`. Pass WithRoot to override.
func New(opts ...Option) (*Reader, error)

func WithRoot(root string) Option

// Read returns the content of relPath, resolved against Root. Returns
// an error if relPath is absolute, escapes Root through `..`, or
// can't be read.
func (r *Reader) Read(ctx context.Context, relPath string) ([]byte, error)

// Exists reports whether relPath exists under Root. Useful for static
// presence checks (linters, validators) that don't need the content.
func (r *Reader) Exists(ctx context.Context, relPath string) (bool, error)
```

The reader treats `relPath` as a slash-separated path that always resolves against `Root`. Absolute paths fail. A path that traverses out of `Root` through `..` also fails before any disk access occurs, so a malicious or buggy caller can't read files outside the intended scope.

## Caching

The package does no caching. File reads through the OS already run cheaply, and the OS page cache handles repeated reads of the same file in the same process. A consumer that reads the same file many times in a tight loop can wrap the reader with its own cache, but doing so rarely pays off.

## Configuration

The package reads no configuration. Construction-time options carry the only knobs:

- `WithRoot` overrides the default git-toplevel root for tests and for tools that operate on a specific subtree.

## Consumers

- `tools/contexttemplate`'s `readFile` template func calls into this package for every static-file include from a template.
- `tools/contexttemplate`'s `typesscopes` registry reads `.commitlint.yaml` to derive Conventional Commits types and scopes.
- Future tooling under `tools/` that needs to read repo-local files without re-doing root resolution itself.

## Out of scope

- **Write operations.** Read-only by design.
- **Template parsing.** This package returns raw bytes. Template parsing happens in the consumer, where the funcmap, locals, and other render-time context live.
- **Glob expansion.** The reader takes one path at a time. Walking a directory or matching a glob falls to the consumer.
- **Caching.** OS-level file caching covers current callers.
