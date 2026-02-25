package core

// Error cost constants matching C tree-sitter.
const (
	ErrorCostPerRecovery    = 500
	ErrorCostPerMissingTree = 110
	ErrorCostPerSkippedTree = 100
	ErrorCostPerSkippedLine = 30
	ErrorCostPerSkippedChar = 1

	MaxVersionCount   = 6
	MaxCostDifference = 18 * ErrorCostPerSkippedTree // = 1800, matches C master
	MaxSummaryDepth   = 16
)
