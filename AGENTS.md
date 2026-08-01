# Agent instructions

Guidance for AI coding agents working in this repository. Read it alongside the per-tool documentation and any memory files the harness loads.

## Commit messages

Draft every commit message in a repo-root `COMMIT_AGENTMSG` file before you run `git commit`. A gitignore entry keeps that file out of history, so it serves purely as a scratchpad for iterating on the message. The workflow runs as follows.

1. Write the full message (subject, body, and trailers) to `COMMIT_AGENTMSG`.
2. Run `just lint-commit-msg` and resolve whatever it reports.
3. Commit the validated draft with `git commit -F COMMIT_AGENTMSG`.

`just lint-commit-msg` mirrors the commit-msg hook by running vale under the commit scope (which catches AI commit tells via `ai-tells-commits`), cspell with the commit dictionary, commitlint for the Conventional Commits shape, and commit-trailers for trailer order. Running it while drafting surfaces problems early, rather than at the commit-msg hook where a late failure interrupts the commit.

The prek commit-msg hook on `.git/COMMIT_EDITMSG` stays the real gate. `COMMIT_AGENTMSG` and its recipe only preview that gate, so a clean recipe run predicts a clean commit but never replaces the hook.

## Prose lint output

The toolchain already defaults to the proofhouse-agent output template: `just lint-prose` and the prek vale hook both pass `--output=proofhouse-agent.tmpl`, so add that flag only when invoking vale directly. The template, synced from the shared `proofhouse` vale package, prints one self-contained line per finding (location, severity, rule, the exact matched text, and the replacement parameter when the rule defines one) plus a totals line, so you can apply fixes without re-reading context through separate commands. Empty output means a clean run, and the exit code carries the result.
