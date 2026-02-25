package treesitter

import "github.com/treesitter-go/treesitter/internal/core"

const (
	ErrorCostPerRecovery    = core.ErrorCostPerRecovery
	ErrorCostPerMissingTree = core.ErrorCostPerMissingTree
	ErrorCostPerSkippedTree = core.ErrorCostPerSkippedTree
	ErrorCostPerSkippedLine = core.ErrorCostPerSkippedLine
	ErrorCostPerSkippedChar = core.ErrorCostPerSkippedChar

	MaxVersionCount   = core.MaxVersionCount
	MaxCostDifference = core.MaxCostDifference
	MaxSummaryDepth   = core.MaxSummaryDepth
)
