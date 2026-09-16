package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataflowDiagnostics_Records(t *testing.T) {
	d := newDataflowDiagnostics("rule-a", "title-a", "prog-a")
	require.Equal(t, "rule-a", d.ruleName)
	require.Equal(t, "prog-a", d.programName)

	// With no counters touched, a limit-hit report must still be safe.
	require.Equal(t, int64(0), d.objectsExpanded.Load())
	require.Equal(t, int64(0), d.memberPairsExpanded.Load())

	d.recordObjectExpansion(7)
	d.recordObjectExpansion(3)
	require.Equal(t, int64(2), d.objectsExpanded.Load())
	require.Equal(t, int64(10), d.memberPairsExpanded.Load())

	d.recordCallFanout(4)
	require.Equal(t, int64(4), d.callFanouts.Load())
}

func TestOperationConfig_RecordHelpersAreNilSafe(t *testing.T) {
	// No diagnostics attached: the helpers must be no-ops, not panics.
	var cfg *OperationConfig
	require.NotPanics(t, func() { cfg.recordObjectExpansion(5) })
	require.NotPanics(t, func() { cfg.recordCallFanout(5) })

	cfg = &OperationConfig{}
	require.NotPanics(t, func() { cfg.recordObjectExpansion(5) })
	require.NotPanics(t, func() { cfg.recordCallFanout(5) })
}

func TestDataflowDiagnostics_WithOption(t *testing.T) {
	d := newDataflowDiagnostics("rule-b", "title-b", "prog-b")
	cfg := NewOperations(WithDataflowDiagnostics(d))
	require.NotNil(t, cfg.diag)
	require.Equal(t, "rule-b", cfg.diag.ruleName)
}
