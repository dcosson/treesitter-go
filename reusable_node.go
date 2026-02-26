package treesitter

// reusable_node.go re-exports ReusableNode from internal/tree.

import itree "github.com/treesitter-go/treesitter/internal/tree"

// ReusableNode walks an old tree in document order, providing subtree
// candidates for reuse during incremental parsing.
type ReusableNode = itree.ReusableNode

// NewReusableNode creates a ReusableNode rooted at the given subtree.
var NewReusableNode = itree.NewReusableNode
