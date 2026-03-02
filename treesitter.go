// Package treesitter is the public API facade for the tree-sitter Go runtime.
// All implementation lives in internal packages; this file re-exports the
// public types, constructors, and accessors.
package treesitter

import (
	"github.com/dcosson/treesitter-go/internal/core"
	iq "github.com/dcosson/treesitter-go/internal/query"
	st "github.com/dcosson/treesitter-go/internal/subtree"
	itree "github.com/dcosson/treesitter-go/internal/tree"
	"github.com/dcosson/treesitter-go/language"
	plex "github.com/dcosson/treesitter-go/lexer"
)

// ---------------------------------------------------------------------------
// Core types (from internal/core)
// ---------------------------------------------------------------------------

type Symbol = core.Symbol

const (
	SymbolEnd         Symbol = core.SymbolEnd
	SymbolError       Symbol = core.SymbolError
	SymbolErrorRepeat Symbol = core.SymbolErrorRepeat
)

type StateID = core.StateID
type FieldID = core.FieldID
type Point = core.Point
type Range = core.Range
type Length = core.Length

var LengthZero = core.LengthZero

func LengthAdd(a, b Length) Length { return core.LengthAdd(a, b) }
func LengthSub(a, b Length) Length { return core.LengthSub(a, b) }

type InputEdit = core.InputEdit
type SymbolMetadata = core.SymbolMetadata

const LexStateNoLookahead uint16 = core.LexStateNoLookahead

type LexMode = core.LexMode
type FieldMapSlice = core.FieldMapSlice
type FieldMapEntry = core.FieldMapEntry
type ParseActionType = core.ParseActionType

const (
	ParseActionTypeHeader  ParseActionType = core.ParseActionTypeHeader
	ParseActionTypeShift   ParseActionType = core.ParseActionTypeShift
	ParseActionTypeReduce  ParseActionType = core.ParseActionTypeReduce
	ParseActionTypeAccept  ParseActionType = core.ParseActionTypeAccept
	ParseActionTypeRecover ParseActionType = core.ParseActionTypeRecover
)

type ParseActionEntry = core.ParseActionEntry
type TableEntry = core.TableEntry

// ---------------------------------------------------------------------------
// Language types (from language/)
// ---------------------------------------------------------------------------

type Language = language.Language
type ExternalScanner = language.ExternalScanner
type ExternalScannerFactory = language.ExternalScannerFactory

// ---------------------------------------------------------------------------
// Lexer types (from lexer/)
// ---------------------------------------------------------------------------

type Input = plex.Input
type StringInput = plex.StringInput
type Lexer = plex.Lexer

func NewStringInput(data []byte) *StringInput { return plex.NewStringInput(data) }
func NewLexer() *Lexer                        { return plex.NewLexer() }

// ---------------------------------------------------------------------------
// Subtree types (from internal/subtree)
// ---------------------------------------------------------------------------

type Subtree = st.Subtree
type SubtreeFlags = st.SubtreeFlags
type FirstLeaf = st.FirstLeaf
type SubtreeHeapData = st.SubtreeHeapData
type SubtreeArena = st.SubtreeArena
type SubtreeID = st.SubtreeID

var SubtreeZero = st.SubtreeZero

var (
	NewSubtreeArena  = st.NewSubtreeArena
	NewLeafSubtree   = st.NewLeafSubtree
	NewNodeSubtree   = st.NewNodeSubtree
	NewInlineSubtree = st.NewInlineSubtree
	SubtreeCanInline = st.SubtreeCanInline
)

var (
	GetSymbol                     = st.GetSymbol
	GetParseState                 = st.GetParseState
	GetPadding                    = st.GetPadding
	GetSize                       = st.GetSize
	GetTotalBytes                 = st.GetTotalBytes
	GetChildCount                 = st.GetChildCount
	IsVisible                     = st.IsVisible
	IsNamed                       = st.IsNamed
	GetIsKeyword                  = st.GetIsKeyword
	IsExtra                       = st.IsExtra
	IsVisibleInContext            = st.IsVisibleInContext
	IsNamedInContext              = st.IsNamedInContext
	StructuralHash                = st.StructuralHash
	IsMissing                     = st.IsMissing
	HasChanges                    = st.HasChanges
	IsFragileLeft                 = st.IsFragileLeft
	IsFragileRight                = st.IsFragileRight
	DependsOnColumn               = st.DependsOnColumn
	GetChildren                   = st.GetChildren
	GetFirstLeaf                  = st.GetFirstLeaf
	GetLeafSymbol                 = st.GetLeafSymbol
	GetVisibleChildCount          = st.GetVisibleChildCount
	GetNamedChildCount            = st.GetNamedChildCount
	GetVisibleDescendantCount     = st.GetVisibleDescendantCount
	GetErrorCost                  = st.GetErrorCost
	GetDynamicPrecedence          = st.GetDynamicPrecedence
	GetRepeatDepth                = st.GetRepeatDepth
	HasExternalScannerStateChange = st.HasExternalScannerStateChange
	GetProductionID               = st.GetProductionID
	GetLookaheadBytes             = st.GetLookaheadBytes
	SubtreeIDOf                   = st.SubtreeIDOf
)

var (
	SetSubtreeSymbol          = st.SetSubtreeSymbol
	SetExtra                  = st.SetExtra
	SetParseState             = st.SetParseState
	HasExternalTokens         = st.HasExternalTokens
	GetExternalScannerState   = st.GetExternalScannerState
	SetExternalScannerState   = st.SetExternalScannerState
	ExternalScannerStateEqual = st.ExternalScannerStateEqual
	SummarizeChildren         = st.SummarizeChildren
	ComputeSizeFromChildren   = st.ComputeSizeFromChildren
)

var EditSubtree = st.EditSubtree
var LengthSaturatingSub = st.LengthSaturatingSub

const (
	SubtreeFlagVisible                       = st.SubtreeFlagVisible
	SubtreeFlagNamed                         = st.SubtreeFlagNamed
	SubtreeFlagExtra                         = st.SubtreeFlagExtra
	SubtreeFlagHasChanges                    = st.SubtreeFlagHasChanges
	SubtreeFlagMissing                       = st.SubtreeFlagMissing
	SubtreeFlagFragileLeft                   = st.SubtreeFlagFragileLeft
	SubtreeFlagFragileRight                  = st.SubtreeFlagFragileRight
	SubtreeFlagHasExternalTokens             = st.SubtreeFlagHasExternalTokens
	SubtreeFlagDependsOnColumn               = st.SubtreeFlagDependsOnColumn
	SubtreeFlagIsKeyword                     = st.SubtreeFlagIsKeyword
	SubtreeFlagHasExternalScannerStateChange = st.SubtreeFlagHasExternalScannerStateChange

	TreeSitterSerializationBufferSize = st.TreeSitterSerializationBufferSize
)

// ---------------------------------------------------------------------------
// Tree types (from internal/tree)
// ---------------------------------------------------------------------------

type Tree = itree.Tree
type Node = itree.Node
type TreeCursorEntry = itree.TreeCursorEntry
type TreeCursor = itree.TreeCursor
type ReusableNode = itree.ReusableNode

var (
	NewTree         = itree.NewTree
	NewTreeCursor   = itree.NewTreeCursor
	NewReusableNode = itree.NewReusableNode
	AdvancePosition = itree.AdvancePosition
)

// ---------------------------------------------------------------------------
// Query types (from internal/query)
// ---------------------------------------------------------------------------

type Query = iq.Query
type QueryError = iq.QueryError
type QueryErrorType = iq.QueryErrorType
type QueryCapture = iq.QueryCapture
type QueryMatch = iq.QueryMatch
type PredicateStepType = iq.PredicateStepType
type PredicateStep = iq.PredicateStep
type QueryCursor = iq.QueryCursor

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

var (
	NewQuery       = iq.NewQuery
	NewQueryCursor = iq.NewQueryCursor
)
