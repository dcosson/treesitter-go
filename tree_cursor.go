package treesitter

// tree_cursor.go re-exports TreeCursor and TreeCursorEntry from internal/tree.

import itree "github.com/treesitter-go/treesitter/internal/tree"

// TreeCursorEntry represents a position in the cursor's traversal stack.
type TreeCursorEntry = itree.TreeCursorEntry

// TreeCursor provides efficient stack-based traversal of a parse tree.
type TreeCursor = itree.TreeCursor

// NewTreeCursor creates a new TreeCursor starting at the given node.
var NewTreeCursor = itree.NewTreeCursor
