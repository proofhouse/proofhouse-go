// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Proofhouse

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
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
// The script reaches bash on stdin rather than as a `-c` argument, so
// the payload never crosses the Windows command-line encoder.
//
// Filtering still applies to the zero byte and the carriage return,
// and both limits sit in bash rather than in the transport. The zero
// byte can't survive any exec boundary. The carriage return disappears when bash on Windows parses
// script text, which holds whether the text arrives as an argument or
// over a pipe; a run with that filter removed returned an empty string
// for a lone `\r` on the windows-2025 job. The round trip asks bash to
// parse a quoted literal, so the parser decides which runes reach the
// other side.
func TestProperty_ShellQuoteRoundTripsThroughBash(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	rapid.Check(t, func(t *rapid.T) {
		s := rapid.String().Filter(func(s string) bool {
			if strings.ContainsRune(s, 0) {
				return false
			}
			if runtime.GOOS == "windows" && strings.ContainsRune(s, '\r') {
				return false
			}
			return true
		}).Draw(t, "s")

		assertRoundTrip(ctx, t, s)
	})
}

// TestShellQuoteRoundTripsKnownWindowsInput pins the input that broke
// the windows-2025 job when the script traveled as a `-c` argument.
// The property check found it once and then lost it, because rapid
// reports the failing iteration's mutated seed rather than the seed
// that reproduces the draw. Naming the bytes here keeps the case under
// test on every platform without waiting for a redraw.
func TestShellQuoteRoundTripsKnownWindowsInput(t *testing.T) {
	t.Parallel()

	// A plain letter, an en quad, a doubled backslash, a plus, a
	// question mark, an astral rune, two more letters, then a run of
	// extended and combining marks.
	known := string([]byte{
		0x41, 0xe2, 0x80, 0x80, 0x5c, 0x5c, 0x2b, 0x3f,
		0xf0, 0x92, 0x90, 0xb9, 0x61, 0x61, 0xc6, 0xbb,
		0xe0, 0xa7, 0xb3, 0xcc, 0x8a,
	})

	assertRoundTrip(t.Context(), t, known)
}

// fatalReporter covers the reporting surface shared by *testing.T and
// *rapid.T. rapid.T carries no Helper method, so the shared assertion
// takes this narrower interface rather than testing.TB.
type fatalReporter interface {
	Fatalf(format string, args ...any)
}

// assertRoundTrip sends one shellQuote result through bash and checks
// that the shell echoes the input back byte-for-byte.
func assertRoundTrip(ctx context.Context, t fatalReporter, s string) {
	cmd := exec.CommandContext(ctx, "bash", "-s")
	cmd.Stdin = strings.NewReader("printf '%s' " + shellQuote(s))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bash exec failed for input %q (quoted: %s): %v", s, shellQuote(s), err)
	}
	if string(out) != s {
		t.Fatalf("round-trip mismatch\n  input:  %q\n  quoted: %s\n  output: %q", s, shellQuote(s), out)
	}
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
