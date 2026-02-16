package treesitter

import (
	"math"
)

// --- Subtree: compact 8-byte value type ---
//
// Subtree is exactly 8 bytes (uint64), matching C tree-sitter's pointer-sized
// Subtree union. The high bit discriminates between two representations:
//
//   Bit 63 = 1: Inline leaf data (remaining 63 bits pack symbol, parse state,
//               padding/size, and flags — no heap allocation needed).
//   Bit 63 = 0: Arena reference (block index + offset within block, referencing
//               a SubtreeHeapData in a SubtreeArena).
//
// This is the foundation data type. Children are stored as []Subtree values
// (not pointers), keeping children contiguous in memory for cache locality.

// Subtree is a compact 8-byte value type representing a parse tree node.
type Subtree struct {
	data uint64
}

const (
	subtreeInlineBit = uint64(1) << 63

	// Inline layout (63 bits available):
	//   [63]    = 1 (inline flag)
	//   [62:55] = symbol (8 bits, 0-255)
	//   [54:39] = parseState (16 bits)
	//   [38:31] = padding bytes (8 bits, 0-255 bytes)
	//   [30:23] = padding column (8 bits, 0-255)
	//   [22:15] = size bytes (8 bits, 0-255 bytes)
	//   [14:7]  = size column (8 bits, 0-255)
	//   [6]     = visible
	//   [5]     = named
	//   [4]     = extra
	//   [3]     = hasChanges
	//   [2]     = isKeyword
	//   [1]     = dependsOnColumn (padding row != 0 — single-bit approximation)
	//   [0]     = unused/reserved
	inlineSymbolShift       = 55
	inlineSymbolMask        = uint64(0xFF) << inlineSymbolShift
	inlineParseStateShift   = 39
	inlineParseStateMask    = uint64(0xFFFF) << inlineParseStateShift
	inlinePaddingBytesShift = 31
	inlinePaddingBytesMask  = uint64(0xFF) << inlinePaddingBytesShift
	inlinePaddingColShift   = 23
	inlinePaddingColMask    = uint64(0xFF) << inlinePaddingColShift
	inlineSizeBytesShift    = 15
	inlineSizeBytesMask     = uint64(0xFF) << inlineSizeBytesShift
	inlineSizeColShift      = 7
	inlineSizeColMask       = uint64(0xFF) << inlineSizeColShift

	inlineVisibleBit        = uint64(1) << 6
	inlineNamedBit          = uint64(1) << 5
	inlineExtraBit          = uint64(1) << 4
	inlineHasChangesBit     = uint64(1) << 3
	inlineIsKeywordBit      = uint64(1) << 2
	inlineDependsOnColBit   = uint64(1) << 1

	// Arena reference layout (63 bits available):
	//   [63]    = 0 (heap flag)
	//   [62:32] = block index (31 bits — supports up to 2 billion blocks)
	//   [31:0]  = offset within block (32 bits)
	arenaBlockShift  = 32
	arenaBlockMask   = uint64(0x7FFFFFFF) << arenaBlockShift
	arenaOffsetMask  = uint64(0xFFFFFFFF)
)

// SubtreeZero is the zero-valued Subtree (nil/empty).
var SubtreeZero = Subtree{}

// subtreeCanInline returns true if a leaf token's data fits in the inline
// representation. Matches C's ts_subtree_can_inline.
func subtreeCanInline(padding, size Length, symbol Symbol, hasExternalTokens bool) bool {
	if hasExternalTokens {
		return false
	}
	if symbol > 255 {
		return false
	}
	// Padding row must be 0 for inline (we only have 8 bits for column).
	if padding.Point.Row > 0 {
		return false
	}
	if padding.Bytes > 255 || padding.Point.Column > 255 {
		return false
	}
	if size.Bytes > 255 || size.Point.Column > 255 {
		return false
	}
	// Size row must be 0 for inline (leaf tokens are single-line).
	if size.Point.Row > 0 {
		return false
	}
	return true
}

// newInlineSubtree creates an inline (leaf) Subtree from the given data.
func newInlineSubtree(symbol Symbol, parseState StateID, padding, size Length, visible, named, extra, isKeyword bool) Subtree {
	bits := subtreeInlineBit
	bits |= uint64(symbol&0xFF) << inlineSymbolShift
	bits |= uint64(parseState) << inlineParseStateShift
	bits |= uint64(padding.Bytes&0xFF) << inlinePaddingBytesShift
	bits |= uint64(padding.Point.Column&0xFF) << inlinePaddingColShift
	bits |= uint64(size.Bytes&0xFF) << inlineSizeBytesShift
	bits |= uint64(size.Point.Column&0xFF) << inlineSizeColShift
	if visible {
		bits |= inlineVisibleBit
	}
	if named {
		bits |= inlineNamedBit
	}
	if extra {
		bits |= inlineExtraBit
	}
	if isKeyword {
		bits |= inlineIsKeywordBit
	}
	return Subtree{data: bits}
}

// newArenaSubtree creates an arena-referenced Subtree from a block index and offset.
func newArenaSubtree(blockIndex uint32, offset uint32) Subtree {
	bits := uint64(blockIndex) << arenaBlockShift
	bits |= uint64(offset)
	// Bit 63 is 0 (heap/arena reference).
	return Subtree{data: bits}
}

// IsZero returns true if this is the zero-valued Subtree.
func (s Subtree) IsZero() bool {
	return s.data == 0
}

// IsInline returns true if this subtree stores inline leaf data (no heap pointer).
func (s Subtree) IsInline() bool {
	return s.data&subtreeInlineBit != 0
}

// arenaBlockIndex returns the block index for an arena-referenced subtree.
func (s Subtree) arenaBlockIndex() uint32 {
	return uint32((s.data & arenaBlockMask) >> arenaBlockShift)
}

// arenaOffset returns the offset within the block for an arena-referenced subtree.
func (s Subtree) arenaOffset() uint32 {
	return uint32(s.data & arenaOffsetMask)
}

// InlineSymbol returns the symbol for an inline subtree.
func (s Subtree) InlineSymbol() Symbol {
	return Symbol((s.data & inlineSymbolMask) >> inlineSymbolShift)
}

// InlineParseState returns the parse state for an inline subtree.
func (s Subtree) InlineParseState() StateID {
	return StateID((s.data & inlineParseStateMask) >> inlineParseStateShift)
}

// InlinePadding returns the padding for an inline subtree.
func (s Subtree) InlinePadding() Length {
	return Length{
		Bytes: uint32((s.data & inlinePaddingBytesMask) >> inlinePaddingBytesShift),
		Point: Point{
			Row:    0, // Inline subtrees have row=0 padding.
			Column: uint32((s.data & inlinePaddingColMask) >> inlinePaddingColShift),
		},
	}
}

// InlineSize returns the size for an inline subtree.
func (s Subtree) InlineSize() Length {
	return Length{
		Bytes: uint32((s.data & inlineSizeBytesMask) >> inlineSizeBytesShift),
		Point: Point{
			Row:    0, // Inline subtrees are single-line.
			Column: uint32((s.data & inlineSizeColMask) >> inlineSizeColShift),
		},
	}
}

// InlineVisible returns true if the inline subtree is visible.
func (s Subtree) InlineVisible() bool {
	return s.data&inlineVisibleBit != 0
}

// InlineNamed returns true if the inline subtree is a named node.
func (s Subtree) InlineNamed() bool {
	return s.data&inlineNamedBit != 0
}

// InlineExtra returns true if the inline subtree is an extra node.
func (s Subtree) InlineExtra() bool {
	return s.data&inlineExtraBit != 0
}

// InlineHasChanges returns true if the inline subtree has pending changes.
func (s Subtree) InlineHasChanges() bool {
	return s.data&inlineHasChangesBit != 0
}

// InlineIsKeyword returns true if the inline subtree is a keyword.
func (s Subtree) InlineIsKeyword() bool {
	return s.data&inlineIsKeywordBit != 0
}

// --- SubtreeHeapData: full heap-allocated subtree node ---

// SubtreeFlags packs boolean flags into a single uint32 bitfield.
type SubtreeFlags uint32

const (
	SubtreeFlagVisible         SubtreeFlags = 1 << 0
	SubtreeFlagNamed           SubtreeFlags = 1 << 1
	SubtreeFlagExtra           SubtreeFlags = 1 << 2
	SubtreeFlagHasChanges      SubtreeFlags = 1 << 3
	SubtreeFlagMissing         SubtreeFlags = 1 << 4
	SubtreeFlagFragileLeft     SubtreeFlags = 1 << 5
	SubtreeFlagFragileRight    SubtreeFlags = 1 << 6
	SubtreeFlagHasExternalTokens SubtreeFlags = 1 << 7
	SubtreeFlagDependsOnColumn SubtreeFlags = 1 << 8
	SubtreeFlagIsKeyword       SubtreeFlags = 1 << 9
)

// FirstLeaf stores the lex-mode-relevant data of the leftmost leaf token.
// Used by the incremental parser to check reusability.
type FirstLeaf struct {
	Symbol     Symbol
	ParseState StateID
}

// SubtreeHeapData contains all fields for a heap-allocated subtree node.
// Field ordering is optimized for cache locality: hot fields first.
type SubtreeHeapData struct {
	// --- Hot fields (accessed on nearly every node visit) ---
	Symbol     Symbol     // 2 bytes
	ParseState StateID    // 2 bytes
	Flags      SubtreeFlags // 4 bytes
	ChildCount uint32     // 4 bytes
	Padding    Length     // 12 bytes (Bytes uint32, Point{Row, Column uint32})
	Size       Length     // 12 bytes

	// --- Children (only for internal nodes) ---
	Children []Subtree   // 24 bytes (slice header)

	// --- Warm fields ---
	VisibleChildCount      uint32
	NamedChildCount        uint32
	VisibleDescendantCount uint32

	// --- Cold fields (accessed during error recovery, incremental parsing) ---
	ErrorCost           uint32
	DynamicPrecedence   int32
	LookaheadBytes      uint32
	RepeatDepth         uint16
	ProductionID        uint16
	FirstLeaf           FirstLeaf
	ExternalScannerState []byte

	// Structural hash (AA-2: hash-consing for O(1) change detection).
	StructuralHash uint32
}

// HasFlag returns true if the given flag is set.
func (d *SubtreeHeapData) HasFlag(f SubtreeFlags) bool {
	return d.Flags&f != 0
}

// SetFlag sets or clears the given flag.
func (d *SubtreeHeapData) SetFlag(f SubtreeFlags, on bool) {
	if on {
		d.Flags |= f
	} else {
		d.Flags &^= f
	}
}

// TotalBytes returns padding + size in bytes.
func (d *SubtreeHeapData) TotalBytes() uint32 {
	return d.Padding.Bytes + d.Size.Bytes
}

// TotalSize returns padding + size as Length.
func (d *SubtreeHeapData) TotalSize() Length {
	return LengthAdd(d.Padding, d.Size)
}

// --- SubtreeArena: batch allocation of SubtreeHeapData ---

const defaultArenaBlockSize = 512

// SubtreeArena allocates SubtreeHeapData structs from contiguous backing
// arrays (blocks). This reduces thousands of individual heap allocations to
// a handful of slice allocations, improving GC performance and cache locality.
type SubtreeArena struct {
	blocks    [][]SubtreeHeapData
	current   int    // index of current block in blocks
	offset    int    // next free slot in current block
	blockSize int    // capacity of each block
}

// NewSubtreeArena creates a new arena with the given block size.
// If blockSize is 0, defaultArenaBlockSize is used.
func NewSubtreeArena(blockSize int) *SubtreeArena {
	if blockSize <= 0 {
		blockSize = defaultArenaBlockSize
	}
	arena := &SubtreeArena{
		blockSize: blockSize,
	}
	arena.addBlock()
	return arena
}

// addBlock adds a new block to the arena.
func (a *SubtreeArena) addBlock() {
	block := make([]SubtreeHeapData, a.blockSize)
	a.blocks = append(a.blocks, block)
	a.current = len(a.blocks) - 1
	a.offset = 0
}

// Alloc allocates a new SubtreeHeapData from the arena and returns a Subtree
// reference to it along with a pointer for initialization.
func (a *SubtreeArena) Alloc() (Subtree, *SubtreeHeapData) {
	if a.offset >= a.blockSize {
		a.addBlock()
	}
	blockIdx := a.current
	offsetIdx := a.offset
	a.offset++
	ptr := &a.blocks[blockIdx][offsetIdx]
	st := newArenaSubtree(uint32(blockIdx), uint32(offsetIdx))
	return st, ptr
}

// Get returns the SubtreeHeapData for an arena-referenced Subtree.
// Panics if the subtree is inline.
func (a *SubtreeArena) Get(s Subtree) *SubtreeHeapData {
	blockIdx := s.arenaBlockIndex()
	offset := s.arenaOffset()
	return &a.blocks[blockIdx][offset]
}

// BlockCount returns the number of blocks in the arena.
func (a *SubtreeArena) BlockCount() int {
	return len(a.blocks)
}

// TotalAllocated returns the total number of SubtreeHeapData structs allocated.
func (a *SubtreeArena) TotalAllocated() int {
	if len(a.blocks) == 0 {
		return 0
	}
	return (len(a.blocks)-1)*a.blockSize + a.offset
}

// --- SubtreeID: unique identity for node comparison ---

// SubtreeID uniquely identifies a subtree for Node identity comparison.
// For inline subtrees, the ID is derived from the raw data bits.
// For arena subtrees, it's the block+offset pair.
type SubtreeID struct {
	Block  uint32
	Offset uint32
	Inline bool
	// For inline subtrees, we store the raw bits for identity.
	InlineBits uint64
}

// SubtreeIDOf returns the SubtreeID for a Subtree.
func SubtreeIDOf(s Subtree) SubtreeID {
	if s.IsInline() {
		return SubtreeID{Inline: true, InlineBits: s.data}
	}
	return SubtreeID{
		Block:  s.arenaBlockIndex(),
		Offset: s.arenaOffset(),
	}
}

// Equal returns true if two SubtreeIDs refer to the same node.
func (id SubtreeID) Equal(other SubtreeID) bool {
	if id.Inline != other.Inline {
		return false
	}
	if id.Inline {
		return id.InlineBits == other.InlineBits
	}
	return id.Block == other.Block && id.Offset == other.Offset
}

// --- Helper functions for creating subtrees ---

// NewLeafSubtree creates a new leaf subtree. If the leaf fits the inline
// representation, it returns an inline Subtree (no arena allocation). Otherwise,
// it allocates from the arena.
func NewLeafSubtree(
	arena *SubtreeArena,
	symbol Symbol,
	padding, size Length,
	parseState StateID,
	hasExternalTokens bool,
	dependsOnColumn bool,
	isKeyword bool,
	lang *Language,
) Subtree {
	meta := lang.SymbolMetadata[symbol]
	visible := meta.Visible
	named := meta.Named

	if subtreeCanInline(padding, size, symbol, hasExternalTokens) {
		return newInlineSubtree(symbol, parseState, padding, size, visible, named, false, isKeyword)
	}

	st, data := arena.Alloc()
	*data = SubtreeHeapData{
		Symbol:     symbol,
		ParseState: parseState,
		Padding:    padding,
		Size:       size,
		FirstLeaf: FirstLeaf{
			Symbol:     symbol,
			ParseState: parseState,
		},
	}
	data.SetFlag(SubtreeFlagVisible, visible)
	data.SetFlag(SubtreeFlagNamed, named)
	data.SetFlag(SubtreeFlagHasExternalTokens, hasExternalTokens)
	data.SetFlag(SubtreeFlagDependsOnColumn, dependsOnColumn)
	data.SetFlag(SubtreeFlagIsKeyword, isKeyword)
	return st
}

// GetSymbol returns the symbol for a subtree, handling both inline and heap.
func GetSymbol(s Subtree, arena *SubtreeArena) Symbol {
	if s.IsInline() {
		return s.InlineSymbol()
	}
	return arena.Get(s).Symbol
}

// GetParseState returns the parse state for a subtree.
func GetParseState(s Subtree, arena *SubtreeArena) StateID {
	if s.IsInline() {
		return s.InlineParseState()
	}
	return arena.Get(s).ParseState
}

// GetPadding returns the padding for a subtree.
func GetPadding(s Subtree, arena *SubtreeArena) Length {
	if s.IsInline() {
		return s.InlinePadding()
	}
	return arena.Get(s).Padding
}

// GetSize returns the size for a subtree.
func GetSize(s Subtree, arena *SubtreeArena) Length {
	if s.IsInline() {
		return s.InlineSize()
	}
	return arena.Get(s).Size
}

// GetTotalBytes returns the total bytes (padding + size) for a subtree.
func GetTotalBytes(s Subtree, arena *SubtreeArena) uint32 {
	if s.IsInline() {
		p := s.InlinePadding()
		sz := s.InlineSize()
		return p.Bytes + sz.Bytes
	}
	return arena.Get(s).TotalBytes()
}

// GetChildCount returns the child count (0 for leaves/inline).
func GetChildCount(s Subtree, arena *SubtreeArena) uint32 {
	if s.IsInline() {
		return 0
	}
	return arena.Get(s).ChildCount
}

// IsVisible returns true if the subtree is visible in the CST.
func IsVisible(s Subtree, arena *SubtreeArena) bool {
	if s.IsInline() {
		return s.InlineVisible()
	}
	return arena.Get(s).HasFlag(SubtreeFlagVisible)
}

// IsNamed returns true if the subtree is a named node.
func IsNamed(s Subtree, arena *SubtreeArena) bool {
	if s.IsInline() {
		return s.InlineNamed()
	}
	return arena.Get(s).HasFlag(SubtreeFlagNamed)
}

// IsExtra returns true if the subtree is an extra node (e.g., whitespace, comments).
func IsExtra(s Subtree, arena *SubtreeArena) bool {
	if s.IsInline() {
		return s.InlineExtra()
	}
	return arena.Get(s).HasFlag(SubtreeFlagExtra)
}

// StructuralHash computes a hash for a subtree for O(1) change detection (AA-2).
func StructuralHash(s Subtree, arena *SubtreeArena) uint32 {
	if s.IsInline() {
		// For inline subtrees, use the lower 32 bits of data as a hash.
		return uint32(s.data & math.MaxUint32)
	}
	return arena.Get(s).StructuralHash
}

// IsMissing returns true if the subtree represents a missing node (error recovery).
func IsMissing(s Subtree, arena *SubtreeArena) bool {
	if s.IsInline() {
		return false
	}
	return arena.Get(s).HasFlag(SubtreeFlagMissing)
}

// HasChanges returns true if the subtree has pending edit changes.
func HasChanges(s Subtree, arena *SubtreeArena) bool {
	if s.IsInline() {
		return s.InlineHasChanges()
	}
	return arena.Get(s).HasFlag(SubtreeFlagHasChanges)
}

// IsFragileLeft returns true if the subtree's left boundary is fragile.
func IsFragileLeft(s Subtree, arena *SubtreeArena) bool {
	if s.IsInline() {
		return false
	}
	return arena.Get(s).HasFlag(SubtreeFlagFragileLeft)
}

// IsFragileRight returns true if the subtree's right boundary is fragile.
func IsFragileRight(s Subtree, arena *SubtreeArena) bool {
	if s.IsInline() {
		return false
	}
	return arena.Get(s).HasFlag(SubtreeFlagFragileRight)
}

// DependsOnColumn returns true if the subtree's parsing depends on column info.
func DependsOnColumn(s Subtree, arena *SubtreeArena) bool {
	if s.IsInline() {
		return s.data&inlineDependsOnColBit != 0
	}
	return arena.Get(s).HasFlag(SubtreeFlagDependsOnColumn)
}

// GetChildren returns the children slice for a heap-allocated subtree.
// Returns nil for inline subtrees.
func GetChildren(s Subtree, arena *SubtreeArena) []Subtree {
	if s.IsInline() {
		return nil
	}
	return arena.Get(s).Children
}

// GetFirstLeaf returns the first leaf data for a subtree.
func GetFirstLeaf(s Subtree, arena *SubtreeArena) FirstLeaf {
	if s.IsInline() {
		return FirstLeaf{
			Symbol:     s.InlineSymbol(),
			ParseState: s.InlineParseState(),
		}
	}
	return arena.Get(s).FirstLeaf
}

// GetVisibleChildCount returns the count of visible children.
func GetVisibleChildCount(s Subtree, arena *SubtreeArena) uint32 {
	if s.IsInline() {
		return 0
	}
	return arena.Get(s).VisibleChildCount
}

// GetNamedChildCount returns the count of named children.
func GetNamedChildCount(s Subtree, arena *SubtreeArena) uint32 {
	if s.IsInline() {
		return 0
	}
	return arena.Get(s).NamedChildCount
}

// GetVisibleDescendantCount returns the count of visible descendants.
func GetVisibleDescendantCount(s Subtree, arena *SubtreeArena) uint32 {
	if s.IsInline() {
		return 0
	}
	return arena.Get(s).VisibleDescendantCount
}

// GetErrorCost returns the error cost for a subtree.
func GetErrorCost(s Subtree, arena *SubtreeArena) uint32 {
	if s.IsInline() {
		return 0
	}
	return arena.Get(s).ErrorCost
}

// GetDynamicPrecedence returns the dynamic precedence for a subtree.
func GetDynamicPrecedence(s Subtree, arena *SubtreeArena) int32 {
	if s.IsInline() {
		return 0
	}
	return arena.Get(s).DynamicPrecedence
}

// GetRepeatDepth returns the repeat depth for a subtree.
func GetRepeatDepth(s Subtree, arena *SubtreeArena) uint16 {
	if s.IsInline() {
		return 0
	}
	return arena.Get(s).RepeatDepth
}

// GetProductionID returns the production ID for a subtree.
func GetProductionID(s Subtree, arena *SubtreeArena) uint16 {
	if s.IsInline() {
		return 0
	}
	return arena.Get(s).ProductionID
}

// GetLookaheadBytes returns the lookahead bytes for a subtree.
func GetLookaheadBytes(s Subtree, arena *SubtreeArena) uint32 {
	if s.IsInline() {
		return 0
	}
	return arena.Get(s).LookaheadBytes
}

// --- Internal node creation ---

// NewNodeSubtree creates a new internal (non-leaf) subtree node with children.
// This is the Go equivalent of C ts_subtree_new_node. The children slice is
// taken ownership of (not copied). After creation, call SummarizeChildren
// to compute aggregate statistics.
func NewNodeSubtree(
	arena *SubtreeArena,
	symbol Symbol,
	children []Subtree,
	productionID uint16,
	lang *Language,
) Subtree {
	meta := lang.SymbolMetadata[symbol]

	st, data := arena.Alloc()
	*data = SubtreeHeapData{
		Symbol:       symbol,
		ChildCount:   uint32(len(children)),
		Children:     children,
		ProductionID: productionID,
	}
	data.SetFlag(SubtreeFlagVisible, meta.Visible)
	data.SetFlag(SubtreeFlagNamed, meta.Named)
	return st
}

// SummarizeChildren computes aggregate statistics from a node's children.
// This mirrors C ts_subtree_summarize_children. It computes:
//   - Padding and Size (from first child's padding and total span)
//   - VisibleChildCount, NamedChildCount, VisibleDescendantCount
//   - FirstLeaf (leftmost leaf's symbol + parse state)
//   - RepeatDepth (for tree balancing of left-recursive repetitions)
//   - ErrorCost (sum of children's error costs)
//   - DynamicPrecedence (sum of children's dynamic precedences)
//   - DependsOnColumn, FragileLeft, FragileRight flags
//   - LookaheadBytes (from rightmost child)
//   - StructuralHash
func SummarizeChildren(s Subtree, arena *SubtreeArena, lang *Language) {
	data := arena.Get(s)
	children := data.Children

	if len(children) == 0 {
		data.Padding = LengthZero
		data.Size = LengthZero
		data.FirstLeaf = FirstLeaf{
			Symbol:     data.Symbol,
			ParseState: data.ParseState,
		}
		return
	}

	var visibleChildCount uint32
	var namedChildCount uint32
	var visibleDescendantCount uint32
	var errorCost uint32
	var dynamicPrecedence int32
	var dependsOnColumn bool
	var structuralHash uint32

	// The node's padding is the first child's padding.
	// The node's size spans from after the first child's padding to the end
	// of the last child.
	firstChildPadding := GetPadding(children[0], arena)
	data.Padding = firstChildPadding

	// Track the cumulative position to compute total size.
	var totalBytes uint32
	for i, child := range children {
		childPadding := GetPadding(child, arena)
		childSize := GetSize(child, arena)

		if i == 0 {
			// First child: its padding becomes this node's padding,
			// so we start counting size from the first child's size.
			totalBytes = childSize.Bytes
		} else {
			totalBytes += childPadding.Bytes + childSize.Bytes
		}

		// Count visible/named children and accumulate descendants.
		childVisible := IsVisible(child, arena)
		childNamed := IsNamed(child, arena)
		childExtra := IsExtra(child, arena)

		if childVisible && !childExtra {
			visibleChildCount++
			if childNamed {
				namedChildCount++
			}
		}

		// Accumulate visible descendant counts.
		if childVisible {
			visibleDescendantCount++
		}
		visibleDescendantCount += GetVisibleDescendantCount(child, arena)

		// Accumulate error costs.
		errorCost += GetErrorCost(child, arena)

		// Accumulate dynamic precedence.
		dynamicPrecedence += GetDynamicPrecedence(child, arena)

		// Track depends_on_column.
		if DependsOnColumn(child, arena) {
			dependsOnColumn = true
		}

		// Accumulate structural hash.
		structuralHash = structuralHash*31 + StructuralHash(child, arena)
	}

	// Compute the total size. We need row/column tracking.
	data.Size = computeSizeFromChildren(children, arena, firstChildPadding)

	// FirstLeaf: walk down the leftmost child to find the leaf.
	data.FirstLeaf = GetFirstLeaf(children[0], arena)

	// RepeatDepth: for left-recursive repetition balancing.
	// If this node and its first child have the same symbol, increment depth.
	if len(children) > 0 {
		firstChildSymbol := GetSymbol(children[0], arena)
		if firstChildSymbol == data.Symbol {
			data.RepeatDepth = GetRepeatDepth(children[0], arena) + 1
		}
	}

	// LookaheadBytes: from the rightmost child.
	lastChild := children[len(children)-1]
	data.LookaheadBytes = GetLookaheadBytes(lastChild, arena)
	if data.LookaheadBytes == 0 {
		// For leaves, lookahead_bytes is the size of the token itself.
		data.LookaheadBytes = GetSize(lastChild, arena).Bytes
	}

	// Fragile flags: inherit from first and last children.
	if len(children) > 0 {
		firstChild := children[0]
		lastChild := children[len(children)-1]

		// Left boundary is fragile if the first child is visible and fragile-left,
		// or if the first child is not visible (hidden node boundary).
		if !IsVisible(firstChild, arena) || IsFragileLeft(firstChild, arena) {
			data.SetFlag(SubtreeFlagFragileLeft, true)
		}
		if !IsVisible(lastChild, arena) || IsFragileRight(lastChild, arena) {
			data.SetFlag(SubtreeFlagFragileRight, true)
		}
	}

	// Error cost for error/missing nodes.
	if data.Symbol == SymbolError {
		errorCost += errorCostPerRecovery + errorCostPerSkippedChar*data.Size.Bytes
		// Count skipped lines.
		if data.Size.Point.Row > 0 {
			errorCost += errorCostPerSkippedLine * data.Size.Point.Row
		}
	}

	data.VisibleChildCount = visibleChildCount
	data.NamedChildCount = namedChildCount
	data.VisibleDescendantCount = visibleDescendantCount
	data.ErrorCost = errorCost
	data.DynamicPrecedence = dynamicPrecedence
	data.StructuralHash = structuralHash
	data.SetFlag(SubtreeFlagDependsOnColumn, dependsOnColumn)
}

// Error cost constants matching C tree-sitter.
const (
	errorCostPerRecovery    = 500
	errorCostPerMissingTree = 110
	errorCostPerSkippedTree = 100
	errorCostPerSkippedLine = 30
	errorCostPerSkippedChar = 1
)

// computeSizeFromChildren calculates the total size (with row/col tracking)
// by walking the children from the second child onward.
func computeSizeFromChildren(children []Subtree, arena *SubtreeArena, firstChildPadding Length) Length {
	if len(children) == 0 {
		return LengthZero
	}

	// Start from the first child's size (its padding is the node's padding,
	// so only its size contributes to the node's size).
	result := GetSize(children[0], arena)

	// Add subsequent children (their padding + size).
	for i := 1; i < len(children); i++ {
		childPadding := GetPadding(children[i], arena)
		childSize := GetSize(children[i], arena)
		result = LengthAdd(result, childPadding)
		result = LengthAdd(result, childSize)
	}

	return result
}

// SetExtra marks a subtree as extra (e.g., comments, whitespace).
// Only works on heap-allocated subtrees.
func SetExtra(s Subtree, arena *SubtreeArena) {
	if s.IsInline() {
		return
	}
	arena.Get(s).SetFlag(SubtreeFlagExtra, true)
}

// SetParseState sets the parse state on a heap-allocated subtree.
func SetParseState(s Subtree, arena *SubtreeArena, state StateID) {
	if s.IsInline() {
		return
	}
	arena.Get(s).ParseState = state
}
