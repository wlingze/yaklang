package ssaapi

import (
	"context"

	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

type OperationConfig struct {
	// 限制递归深度，每一次递归核心函数，计数器都会加一
	// 上下文计数器受到这个限制
	MaxDepth int
	MinDepth int

	// Hook
	HookEveryNode        []func(*Value) error
	UntilNode            func(*Value) bool
	AllowIgnoreCallStack bool
	ctx                  context.Context

	// workBudget bounds total node visits across ALL sources in one dataflow
	// analysis (getTopDefs/getBottomUses). Unlike recursiveCounter (per-source,
	// reset each GetTopDefs call), this is shared across the per-source loop in
	// Values.GetTopDefs so a heavy rule matching tens of thousands of sources
	// can't do N×5000 node visits. Set via WithExclusiveWorkBudget from
	// DataFlowWithSFConfig (which reads it off the sfvm.Config). nil = no budget.
	workBudget *sf.RuleWorkBudget

	//用来记录上一次的值
	lastValue *Value

	programOverLay *ProgramOverLay // for program overlay analysis
	structBound    *structBound

	// diag carries rule-level context (rule name, program name) into the
	// recursive descent so limit-hit warnings can attribute the blow-up to the
	// rule that triggered it. nil when the caller did not supply it.
	diag *dataflowDiagnostics
}

type OperationOption func(*OperationConfig)

func WithMaxDepth(maxDepth int) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.MaxDepth = maxDepth
	}
}

func WithProgramOverlay(programOverLay *ProgramOverLay) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.programOverLay = programOverLay
	}
}

func WithLastValue(value *Value) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.lastValue = value
	}
}

func WithMinDepth(minDepth int) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.MinDepth = minDepth
	}
}

func WithAllowCallStack(allowCallStack bool) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.AllowIgnoreCallStack = allowCallStack
	}
}

func WithDepthLimit(depth int) OperationOption {
	return func(operationConfig *OperationConfig) {
		if depth > 0 {
			operationConfig.MaxDepth = depth
			operationConfig.MinDepth = -depth
			return
		}
		operationConfig.MaxDepth = -depth
		operationConfig.MinDepth = depth
	}
}

func WithHookEveryNode(hookNode func(*Value) error) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.HookEveryNode = append(operationConfig.HookEveryNode, hookNode)
	}
}

func WithUntilNode(untilNode func(*Value) bool) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.UntilNode = untilNode
	}
}

func WithExclusiveContext(ctx context.Context) OperationOption {
	return func(operationConfig *OperationConfig) {
		if operationConfig.ctx != nil {
			operationConfig.ctx = ctx
		}
	}
}

// WithExclusiveWorkBudget attaches the per-rule total-work budget to the
// dataflow OperationConfig so AnalyzeContext.check() can increment it per node
// visited across all sources (Values.GetTopDefs per-source loop shares one
// budget). nil budget is a no-op.
func WithExclusiveWorkBudget(b *sf.RuleWorkBudget) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.workBudget = b
	}
}

// WithDataflowDiagnostics attaches rule-level context (rule name, program name)
// to the dataflow config so limit-hit warnings can report which rule and
// project caused the blow-up. See dataflowDiagnostics.
func WithDataflowDiagnostics(d *dataflowDiagnostics) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.diag = d
	}
}

// recordObjectExpansion notes that an object's full member set was expanded
// during traversal. A no-op when no diagnostics are attached.
func (c *OperationConfig) recordObjectExpansion(pairs int) {
	if c == nil {
		return
	}
	c.diag.recordObjectExpansion(pairs)
}

// recordObjectExpanded notes that a specific object was expanded, so repeated
// widening of the same object can be detected. A no-op without diagnostics.
func (c *OperationConfig) recordObjectExpanded(id int64) {
	if c == nil {
		return
	}
	c.diag.recordObjectExpanded(id)
}

// recordCallFanout notes that a call/return site fanned out over N callers.
// A no-op when no diagnostics are attached.
func (c *OperationConfig) recordCallFanout(n int) {
	if c == nil {
		return
	}
	c.diag.recordCallFanout(n)
}

// reportLimitHit emits the appended diagnostic line for a limit hit. It is a
// no-op when no diagnostics are attached, so the original warning remains the
// only output in that case.
func (c *OperationConfig) reportLimitHit(direct AnalysisType, resultCount int) {
	if c == nil {
		return
	}
	c.diag.reportLimitHit(direct, resultCount)
}

func NewOperations(opt ...OperationOption) *OperationConfig {
	config := &OperationConfig{
		MaxDepth:             500,
		MinDepth:             -500,
		AllowIgnoreCallStack: true,
		ctx:                  context.Background(),
	}

	for _, o := range opt {
		o(config)
	}
	return config
}

func FullUseDefChain(value *Value, opts ...OperationOption) *Value {
	value.GetTopDefs(opts...)
	value.GetBottomUses()
	return value
}
