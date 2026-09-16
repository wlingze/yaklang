package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// wideStructViaCall is the realistic fan-out shape: the struct is created in
// one function, handed to another as a pointer parameter, and that callee reads
// exactly ONE field. Tracing the read site has to come back to the Make through
// the parameter member path, carrying the requested key with it.
const wideStructViaCall = `package main

type Wide struct {
	a string
	b string
	c string
	d string
	e string
	f string
	g string
	h string
}

func consume(any) {}

func read(holder *Wide) {
	consume(holder.a)
}

func build(cmd string) {
	w := &Wide{}
	w.a = cmd
	w.b = "b"
	w.c = "c"
	w.d = "d"
	w.e = "e"
	w.f = "f"
	w.g = "g"
	w.h = "h"
	read(w)
}
`

// TestMemberFanout_CallPathKeepsSingleKey pins the good case: when a topdef
// trace follows a real call path, the requested field resolves directly to its
// assignment and the wide object is NEVER enumerated.
//
// This is the behaviour to preserve. It is also the contrast that shows the
// fan-out is not inherent to member tracing: it only appears when the trace
// starts from a bare object with no key constraint.
func TestMemberFanout_CallPathKeepsSingleKey(t *testing.T) {
	prog, err := Parse(wideStructViaCall, WithLanguage(ssaconfig.GO))
	require.NoError(t, err)
	require.NotNil(t, prog)

	res, err := prog.SyntaxFlowWithError(`consume(* as $target)`)
	require.NoError(t, err)
	targets := res.GetValues("target")
	require.NotEmpty(t, targets, "expected consume() argument to match")

	diag := newDataflowDiagnostics("member-fanout-callpath", "callpath fanout", prog.GetProgramName())
	var allDefs Values
	for _, tv := range targets {
		defs := tv.GetTopDefs(WithDataflowDiagnostics(diag))
		allDefs = append(allDefs, defs...)
	}

	expanded := diag.objectsExpanded.Load()
	pairs := diag.memberPairsExpanded.Load()
	t.Logf("call-path trace: objectsExpanded=%d memberPairsExpanded=%d (struct has 8 fields)", expanded, pairs)
	for _, d := range allDefs {
		t.Logf("  def opcode=%s value=%s", d.GetOpcode(), d.String())
	}

	require.Equal(t, int64(0), expanded,
		"a call-path trace must resolve the requested field without enumerating the object")
	require.Equal(t, int64(0), pairs,
		"no sibling fields should be pulled in when a real call path exists")

	// The resolved definition must be the parameter that was assigned into the
	// field, proving the trace followed the field and not the whole object.
	found := false
	for _, d := range allDefs {
		if d.GetOpcode() == "Parameter" {
			found = true
			break
		}
	}
	require.True(t, found, "expected a parameter definition for the traced field")
}
