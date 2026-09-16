package ssaapi

import (
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

// ruleNameForDiagnostics returns the rule's name for diagnostics, tolerating a
// nil result/rule (dataflow can be driven directly without a rule).
func ruleNameForDiagnostics(sfResult *sfvm.SFFrameResult) string {
	if sfResult == nil {
		return ""
	}
	rule := sfResult.GetRule()
	if rule == nil {
		return ""
	}
	return rule.RuleName
}

func ruleTitleForDiagnostics(sfResult *sfvm.SFFrameResult) string {
	if sfResult == nil {
		return ""
	}
	rule := sfResult.GetRule()
	if rule == nil {
		return ""
	}
	return rule.Title
}

// dataflowProgramName returns the program name for the value being analyzed.
func dataflowProgramName(value *Value) string {
	if value == nil || value.ParentProgram == nil {
		return ""
	}
	return value.ParentProgram.GetProgramName()
}

// dataflowDiagnostics carries rule-level context plus per-descent counters so a
// limit hit can be attributed to a concrete rule, program, and analysis entry
// point instead of emitting a bare "too many values" warning.
//
// The counters are deliberately coarse (atomic int64) because the descent runs
// hot; they exist to answer "what blew up", not to build a precise profile.
type dataflowDiagnostics struct {
	ruleName    string
	ruleTitle   string
	programName string

	// entryValue is the value the analysis started from. Recorded once; used to
	// show which source produced the oversized result set.
	entryValue atomic.Value

	// objectsExpanded counts how many times an object's full member set was
	// expanded during traversal (visitUserFallback's GetAllMember path). This is
	// the counter that reveals Object.Print(J.a) fan-out: expanding an object
	// pulls in every member, each of which is then traversed.
	objectsExpanded atomic.Int64
	// memberPairsExpanded counts individual member entries pulled in by those
	// expansions, i.e. the real fan-out factor.
	memberPairsExpanded atomic.Int64
	// callFanouts counts how many times a call/return site fanned out over
	// callers (GetCalledBy) instead of following a single resolved edge.
	callFanouts atomic.Int64

	// expandedObjectIDs records which objects were expanded in this descent.
	// When the same object is expanded repeatedly, the recursion is redoing
	// identical work, which is a behaviour-preserving place to optimize. Stored
	// behind a mutex because a descent may fan out across goroutines.
	expandedMu  sync.Mutex
	expandedIDs map[int64]int

	// recursionEvents counts getTopDefs/getBottomUses entries, so the ratio of
	// expansions to traversals shows how much of the work is object widening.
	recursionEvents atomic.Int64
}

func newDataflowDiagnostics(ruleName, ruleTitle, programName string) *dataflowDiagnostics {
	return &dataflowDiagnostics{
		ruleName:    ruleName,
		ruleTitle:   ruleTitle,
		programName: programName,
		expandedIDs: make(map[int64]int),
	}
}

func (d *dataflowDiagnostics) setEntryValue(v *Value) {
	if d == nil || v == nil {
		return
	}
	if d.entryValue.Load() != nil {
		return
	}
	d.entryValue.Store(v)
}

func (d *dataflowDiagnostics) entryValueString() string {
	if d == nil {
		return ""
	}
	v, _ := d.entryValue.Load().(*Value)
	if v == nil {
		return ""
	}
	return v.StringWithRange()
}

// recordObjectExpansion notes that an object's member set was expanded. pairs
// is the number of member entries pulled in by this expansion.
func (d *dataflowDiagnostics) recordObjectExpansion(pairs int) {
	if d == nil {
		return
	}
	d.objectsExpanded.Add(1)
	d.memberPairsExpanded.Add(int64(pairs))
}

// recordObjectExpanded notes that a specific object was expanded, tracking how
// many times. A high repeat count means the same widening is being redone.
func (d *dataflowDiagnostics) recordObjectExpanded(id int64) {
	if d == nil || id <= 0 {
		return
	}
	d.expandedMu.Lock()
	d.expandedIDs[id]++
	d.expandedMu.Unlock()
}

// repeatedExpansions returns how many expansions were repeats of an object that
// had already been expanded in this descent.
func (d *dataflowDiagnostics) repeatedExpansions() int64 {
	if d == nil {
		return 0
	}
	d.expandedMu.Lock()
	defer d.expandedMu.Unlock()
	var repeats int64
	for _, n := range d.expandedIDs {
		if n > 1 {
			repeats += int64(n - 1)
		}
	}
	return repeats
}

// distinctObjectsExpanded returns how many different objects were expanded.
func (d *dataflowDiagnostics) distinctObjectsExpanded() int {
	if d == nil {
		return 0
	}
	d.expandedMu.Lock()
	defer d.expandedMu.Unlock()
	return len(d.expandedIDs)
}

func (d *dataflowDiagnostics) recordCallFanout(n int) {
	if d == nil {
		return
	}
	d.callFanouts.Add(int64(n))
}

// reportLimitHit logs a single structured warning explaining which rule and
// program hit which limit, what the analysis started from, and what dominated
// the fan-out. Called from the GetTopDefs / GetBottomUses size guards.
func (d *dataflowDiagnostics) reportLimitHit(direct AnalysisType, resultCount int) {
	if d == nil {
		// No diagnostics attached: stay silent so callers that did not opt in
		// keep exactly the original log output.
		return
	}
	log.Warnf(
		"SSA dataflow limit hit: rule=%q title=%q program=%q direct=%s result=%d limit=%d "+
			"objectsExpanded=%d distinctObjects=%d repeatedExpansions=%d memberPairsExpanded=%d callFanouts=%d entry=%s",
		d.ruleName, d.ruleTitle, d.programName, direct, resultCount, dataflowValueLimit,
		d.objectsExpanded.Load(), d.distinctObjectsExpanded(), d.repeatedExpansions(),
		d.memberPairsExpanded.Load(), d.callFanouts.Load(),
		d.entryValueString(),
	)
}
