package treesitter

// query.go re-exports Query types from internal/query.

import iq "github.com/treesitter-go/treesitter/internal/query"

type Query = iq.Query
type QueryError = iq.QueryError
type QueryErrorType = iq.QueryErrorType
type QueryCapture = iq.QueryCapture
type QueryMatch = iq.QueryMatch
type PredicateStepType = iq.PredicateStepType
type PredicateStep = iq.PredicateStep

const (
	QueryErrorNone      = iq.QueryErrorNone
	QueryErrorSyntax    = iq.QueryErrorSyntax
	QueryErrorNodeType  = iq.QueryErrorNodeType
	QueryErrorField     = iq.QueryErrorField
	QueryErrorCapture   = iq.QueryErrorCapture
	QueryErrorStructure = iq.QueryErrorStructure
	QueryErrorLanguage  = iq.QueryErrorLanguage

	PredicateStepDone    = iq.PredicateStepDone
	PredicateStepCapture = iq.PredicateStepCapture
	PredicateStepString  = iq.PredicateStepString
)

var NewQuery = iq.NewQuery
