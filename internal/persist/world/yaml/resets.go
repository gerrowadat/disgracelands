// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import "github.com/gerrowadat/disgracelands/internal/game"

// Reset chains, §4.4 of docs/design/data-format.md: the flat opcode list
// with its bare IfFlag becomes a list of top-level commands each carrying a
// `Then` chain, where a run of consecutive IfFlag!=0 commands nests under
// the IfFlag==0 command before them. Flattening reverses this exactly — the
// mapping is a bijection by construction, since IfFlag!=0 on the *first*
// command in a zone is never emitted by the classic writer and, if a file
// somehow has one anyway, NestResets treats it as its own top-level entry
// rather than dropping it.

// ResetNode is one entry in the nested form: a command plus the chain that
// runs only while each previous one keeps succeeding.
type ResetNode struct {
	Command game.ResetCommand
	Then    []ResetNode
}

// NestResets groups a flat command list into chains, porting §4.4's rule.
func NestResets(cmds []game.ResetCommand) []ResetNode {
	var out []ResetNode
	for _, cmd := range cmds {
		if cmd.IfFlag != 0 && len(out) > 0 {
			last := &out[len(out)-1]
			last.Then = append(last.Then, ResetNode{Command: cmd})
			continue
		}
		out = append(out, ResetNode{Command: cmd})
	}
	return out
}

// FlattenResets is NestResets' inverse: a depth-first walk that reproduces
// the original IfFlag values, since nesting depth *is* the IfFlag encoding
// rather than a separate field of ResetNode.
func FlattenResets(nodes []ResetNode) []game.ResetCommand {
	var out []game.ResetCommand
	for _, n := range nodes {
		out = append(out, n.Command)
		for _, child := range n.Then {
			out = append(out, flattenChild(child)...)
		}
	}
	return out
}

// flattenChild forces IfFlag to 1 on the way back out: a child's own
// IfFlag is redundant with its position in Then (that is what makes it a
// child at all), but the flattened form still needs the bit set for the
// classic opcode to mean what it says.
func flattenChild(n ResetNode) []game.ResetCommand {
	cmd := n.Command
	cmd.IfFlag = 1
	out := []game.ResetCommand{cmd}
	for _, child := range n.Then {
		out = append(out, flattenChild(child)...)
	}
	return out
}
