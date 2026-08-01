# `tools/internal/github`

Configured [go-github](https://github.com/google/go-github) client with on-disk HTTP caching via [httpcache](https://github.com/bartventer/httpcache) and credentials resolved from the environment or the `gh` CLI's stored auth. The package returns a ready-to-use `*github.Client` and gets out of the way.

## Purpose

Tools under `tools/` need typed GitHub API access for read-only operations: listing labels, assignees, milestones, projects, validating issue and PR references, looking up commit metadata. Doing this through `exec.Command("gh", "api", ...)` in every consumer means parsing JSON shells, no type safety, and no shared rate-limit story. Doing it through go-github gives every consumer a typed client. Adding the httpcache transport in front of that client means ETag-based conditional requests, 304 responses on repeat calls, and a shared on-disk cache that keeps data available across renders and processes.

The package wraps nothing beyond the constructor. Callers consume the standard go-github API surface directly.

## API

```go
// NewClient returns a go-github client configured with HTTP caching and
// credentials. The client itself is the standard *github.Client from
// google/go-github; this package adds the transport and the token, then
// gets out of the way.
func NewClient(ctx context.Context, opts ...Option) (*github.Client, error)

// WithCacheDir overrides the default disk-cache directory.
func WithCacheDir(dir string) Option

// WithToken supplies an auth token explicitly, overriding the env-var
// and gh-CLI fallback resolution.
func WithToken(token string) Option
```

The returned client supports every read endpoint go-github exposes. Callers paginate, filter, and decode using go-github's own helpers.

## Caching

The httpcache library sits as a transport between go-github and the network. Every GET request goes through the cache: a hit with a still-valid ETag becomes a conditional request that the GitHub API answers with a cheap 304 response, and the cached body returns to the caller without parsing fresh JSON. Cache misses continue on to the live API and write the response back to disk.

By default the cache directory sits at `<repo-root>/tmp/cache/github/`, resolved at construction time via `git rev-parse --show-toplevel`. Callers that want a different location pass `WithCacheDir`.

When a specific call needs fresh data, the caller sends `Cache-Control: no-cache` on that request through go-github's request-customization hooks. The transport honors the header and skips the cache for that one call.

## Auth

The constructor resolves credentials in two steps:

1. The `GITHUB_TOKEN` env var, if present.
2. The output of `gh auth token`, run as a subprocess, as a fallback.

CI environments set the env var. Local devs typically rely on the `gh` CLI's stored auth, so the fallback fires there. Either path produces a token that go-github attaches to outgoing requests.

Auth resolution runs once per constructed client. Consumers that stand up more than one GitHub-backed component construct one client and share it rather than calling `NewClient` per component, which avoids repeating the `gh auth token` subprocess. `tools/contexttemplate` does this through its per-render `deps`: the four `gh*` registries share a single client built lazily on first use.

`WithToken` bypasses both and uses the supplied value directly. Tests typically pass an empty token plus a recorded HTTP transport.

## Consumers

- `tools/contexttemplate`'s `ghlabels`, `ghassignees`, `ghmilestones`, and `ghprojects` registries each call into the GitHub API for the data their section partials render.
- `tools/validate-pr` (planned migration) absorbs its current `gh` CLI shell-outs for issue and user reference validation into this package.

## Out of scope

- **Wrapper methods.** This package doesn't grow `Labels()`, `Assignees()`, or any other typed-wrapper API. Callers use go-github directly.
- **Write operations.** Read-only suffices for current consumers. A scanner mutating GitHub state would need explicit opt-in beyond what this package offers.
- **GraphQL.** Only REST through go-github. Adding GraphQL would mean a different client and a different cache strategy entirely.
- **Rate-limit handling beyond go-github defaults.** httpcache absorbs most of the load, and go-github already surfaces rate-limit metadata on responses.
