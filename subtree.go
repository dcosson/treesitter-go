package treesitter

// subtree.go re-exports types and functions from internal/subtree.
// This maintains backwards compatibility while the implementation lives
// in the internal package. These re-exports will be removed as downstream
// packages are updated to import internal/subtree directly.

import st "github.com/treesitter-go/treesitter/internal/subtree"

// --- Types ---
type Subtree = st.Subtree
type SubtreeFlags = st.SubtreeFlags
type FirstLeaf = st.FirstLeaf
type SubtreeHeapData = st.SubtreeHeapData
type SubtreeArena = st.SubtreeArena
type SubtreeID = st.SubtreeID

// --- Variables ---
var SubtreeZero = st.SubtreeZero

// --- Constructors ---
var NewSubtreeArena = st.NewSubtreeArena
var NewLeafSubtree = st.NewLeafSubtree
var NewNodeSubtree = st.NewNodeSubtree
var NewInlineSubtree = st.NewInlineSubtree
var SubtreeCanInline = st.SubtreeCanInline

// --- Accessor functions ---
var (
	GetSymbol                 = st.GetSymbol
	GetParseState             = st.GetParseState
	GetPadding                = st.GetPadding
	GetSize                   = st.GetSize
	GetTotalBytes             = st.GetTotalBytes
	GetChildCount             = st.GetChildCount
	IsVisible                 = st.IsVisible
	IsNamed                   = st.IsNamed
	GetIsKeyword              = st.GetIsKeyword
	IsExtra                   = st.IsExtra
	IsVisibleInContext        = st.IsVisibleInContext
	IsNamedInContext          = st.IsNamedInContext
	StructuralHash            = st.StructuralHash
	IsMissing                 = st.IsMissing
	HasChanges                = st.HasChanges
	IsFragileLeft             = st.IsFragileLeft
	IsFragileRight            = st.IsFragileRight
	DependsOnColumn           = st.DependsOnColumn
	GetChildren               = st.GetChildren
	GetFirstLeaf              = st.GetFirstLeaf
	GetLeafSymbol             = st.GetLeafSymbol
	GetVisibleChildCount      = st.GetVisibleChildCount
	GetNamedChildCount        = st.GetNamedChildCount
	GetVisibleDescendantCount = st.GetVisibleDescendantCount
	GetErrorCost              = st.GetErrorCost
	GetDynamicPrecedence      = st.GetDynamicPrecedence
	GetRepeatDepth            = st.GetRepeatDepth
	HasExternalScannerStateChange = st.HasExternalScannerStateChange
	GetProductionID           = st.GetProductionID
	GetLookaheadBytes         = st.GetLookaheadBytes
	SubtreeIDOf               = st.SubtreeIDOf
)

// --- Mutator functions ---
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

// --- Constants ---
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
