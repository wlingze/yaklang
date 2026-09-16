package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// wideStructGoProgram is a Go program whose struct carries many fields while
// only ONE of them is ever read. It is the minimal shape that exposes
// object/member fan-out: tracing the read site must not require walking every
// sibling field.
const wideStructGoProgram = `package main

type Wide struct {
	a string
	b string
	c string
	d string
	e string
	f string
	g string
	h string
	i string
	j string
}

func consume(any) {}

func run(cmd string) {
	w := &Wide{}
	w.a = cmd
	w.b = "b"
	w.c = "c"
	w.d = "d"
	w.e = "e"
	w.f = "f"
	w.g = "g"
	w.h = "h"
	w.i = "i"
	w.j = "j"
	consume(w.a)
}
`

func parseWideStructProgram(t *testing.T) *Program {
	t.Helper()
	prog, err := Parse(wideStructGoProgram, WithLanguage(ssaconfig.GO))
	require.NoError(t, err)
	require.NotNil(t, prog)
	return prog
}

// consumeMember returns the object whose member is read at the consume() call
// site, i.e. the `w` in `consume(w.a)`.
func consumeMemberObject(t *testing.T, prog *Program) *Value {
	t.Helper()
	res, err := prog.SyntaxFlowWithError(`consume(* as $target)`)
	require.NoError(t, err)
	targets := res.GetValues("target")
	require.NotEmpty(t, targets, "expected consume() argument to match")
	for _, tv := range targets {
		if tv.IsMember() && tv.GetObject() != nil {
			return tv.GetObject()
		}
	}
	t.Fatal("no member object found at the consume() call site")
	return nil
}

// TestMemberFanout_TopDefExpandsWholeObject documents and guards the fan-out
// behaviour of a topdef trace over an object.
//
// The struct has 10 fields but only `w.a` is ever read. Tracing the object must
// not enumerate all 10 sibling fields: doing so is what makes wide objects
// explode, because each enumerated member is traced recursively in turn.
//
// The assertion is written against the CURRENT (unfixed) behaviour so the
// measurement is visible; it is tightened once traversal stops widening.
func TestMemberFanout_TopDefExpandsWholeObject(t *testing.T) {
	prog := parseWideStructProgram(t)
	obj := consumeMemberObject(t, prog)

	memberCount := len(obj.GetAllMember())
	require.Equal(t, 10, memberCount, "precondition: the struct should expose 10 fields")

	diag := newDataflowDiagnostics("member-fanout", "member fanout", prog.GetProgramName())
	_ = obj.GetTopDefs(WithDataflowDiagnostics(diag))

	expanded := diag.objectsExpanded.Load()
	pairs := diag.memberPairsExpanded.Load()
	t.Logf("tracing one read expanded objects=%d memberPairs=%d (object has %d fields)",
		expanded, pairs, memberCount)

	require.Equal(t, int64(1), expanded,
		"the object should be expanded exactly once per trace")
	require.Equal(t, int64(memberCount), pairs,
		"CURRENT BEHAVIOUR: tracing the object pulls in every sibling field")
}
