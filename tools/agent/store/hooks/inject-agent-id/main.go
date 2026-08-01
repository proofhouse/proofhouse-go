// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Proofhouse

// Command inject-agent-id rewrites Bash tool calls from Claude Code
// subagents so the shell environment carries CLAUDE_CODE_AGENT_ID and
// CLAUDE_CODE_AGENT_TYPE, letting downstream consumers (agentstore,
// ad-hoc scripts) attribute writes to the firing agent. The binary
// runs as a PreToolUse hook. It reads the hook payload from standard
// input and prepends an export statement onto the bash command. The
// rewritten input goes back out on standard output. Every non-rewrite
// path exits 0 with empty stdout, which the PreToolUse hook protocol treats
// as run-the-tool-with-original-input, keeping the agent running
// through unexpected payloads.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
)

const eventPreToolUse = "PreToolUse"

type payload struct {
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
	AgentID       string         `json:"agent_id"`
	AgentType     string         `json:"agent_type"`
}

type hookOutput struct {
	HookSpecificOutput specificOutput `json:"hookSpecificOutput"`
}

type specificOutput struct {
	HookEventName      string         `json:"hookEventName"`
	PermissionDecision string         `json:"permissionDecision"`
	UpdatedInput       map[string]any `json:"updatedInput"`
}

func main() {
	run(os.Stdin, os.Stdout)
}

func run(in io.Reader, out io.Writer) {
	raw, err := io.ReadAll(in)
	if err != nil {
		return
	}
	var p payload
	if err = json.Unmarshal(raw, &p); err != nil {
		return
	}
	if p.HookEventName != eventPreToolUse || p.ToolName != "Bash" || p.AgentID == "" {
		return
	}
	cmd, ok := p.ToolInput["command"].(string)
	if !ok {
		return
	}
	prefix := "export CLAUDE_CODE_AGENT_ID=" + shellQuote(p.AgentID) +
		" CLAUDE_CODE_AGENT_TYPE=" + shellQuote(p.AgentType) + "; "
	updated := make(map[string]any, len(p.ToolInput))
	maps.Copy(updated, p.ToolInput)
	updated["command"] = prefix + cmd
	if err = json.NewEncoder(out).Encode(hookOutput{
		HookSpecificOutput: specificOutput{
			HookEventName:      eventPreToolUse,
			PermissionDecision: "allow",
			UpdatedInput:       updated,
		},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "inject-agent-id:", err)
	}
}

// shellQuote wraps s in single quotes. Each embedded single quote
// becomes a four-character escape sequence: close the literal, emit a
// backslash-escaped quote, reopen. An empty input yields two single
// quotes, the canonical empty literal.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
