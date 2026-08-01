// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Proofhouse

package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_ShellQuoteRoundTripsThroughBash treats bash as ground
// truth. The check generates strings, feeds each through shellQuote,
// hands the result to bash through printf, then asserts the shell
// echoes the input back byte-for-byte. This exposes corners of
// single-quote escaping (embedded quotes, newlines, dollar signs,
// backslashes) that example-based tests miss.
//
// The script reaches bash on stdin rather than as a `-c` argument.
// Passing it as an argument makes Windows re-encode the command line
// and decode it again inside the bash runtime. Bytes get dropped in
// that round trip. The carriage return once needed a Windows-only
// filter for that reason. A later sweep turned up a second input
// class the filter never covered. Handing the script over a pipe
// skips the re-encode, so one generator now serves every platform.
//
// The zero byte stays filtered. That limit belongs to the contract
// rather than to any transport, since a Go string holding one can't
// survive an exec boundary.
func TestProperty_ShellQuoteRoundTripsThroughBash(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Filter(func(s string) bool {
			return !strings.ContainsRune(s, 0)
		}).Draw(t, "s")

		cmd := exec.CommandContext(ctx, "bash", "-s")
		cmd.Stdin = strings.NewReader("printf '%s' " + shellQuote(s))
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("bash exec failed for input %q (quoted: %s): %v", s, shellQuote(s), err)
		}
		if string(out) != s {
			t.Fatalf("round-trip mismatch\n  input:  %q\n  quoted: %s\n  output: %q", s, shellQuote(s), out)
		}
	})
}

// TestProperty_RewritePreservesOriginalCommand checks the structural
// invariant of the rewrite. For any (agent_id, agent_type, command)
// tuple with non-empty agent_id, the rewritten command equals a prefix
// followed by the original command, where the prefix ends in "; " and
// exports both env vars through shellQuote on each value.
func TestProperty_RewritePreservesOriginalCommand(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		agentID := rapid.StringMatching(`[A-Za-z0-9_-]+`).Draw(t, "agent_id")
		agentType := rapid.String().Filter(func(s string) bool {
			return !strings.ContainsRune(s, 0)
		}).Draw(t, "agent_type")
		original := rapid.String().Filter(func(s string) bool {
			return !strings.ContainsRune(s, 0)
		}).Draw(t, "command")

		payload := map[string]any{
			"hook_event_name": "PreToolUse",
			"tool_name":       "Bash",
			"tool_input":      map[string]any{"command": original},
			"agent_id":        agentID,
			"agent_type":      agentType,
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		var out bytes.Buffer
		run(bytes.NewReader(raw), &out)

		var got hookOutput
		if err = json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal hook output: %v\nraw: %s", err, out.String())
		}
		rewritten, ok := got.HookSpecificOutput.UpdatedInput["command"].(string)
		if !ok {
			t.Fatalf(
				"updatedInput.command missing or not a string: %#v",
				got.HookSpecificOutput.UpdatedInput["command"],
			)
		}

		wantPrefix := "export CLAUDE_CODE_AGENT_ID=" + shellQuote(agentID) +
			" CLAUDE_CODE_AGENT_TYPE=" + shellQuote(agentType) + "; "
		if !strings.HasPrefix(rewritten, wantPrefix) {
			t.Fatalf(
				"rewritten command missing expected prefix\n  rewritten: %q\n  want pfx:  %q",
				rewritten,
				wantPrefix,
			)
		}
		if suffix, want := rewritten[len(wantPrefix):], original; suffix != want {
			t.Fatalf("rewritten suffix != original command\n  suffix: %q\n  want:   %q", suffix, want)
		}
	})
}
