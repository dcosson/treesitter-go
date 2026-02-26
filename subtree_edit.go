package treesitter

// subtree_edit.go re-exports edit functions from internal/subtree.

import st "github.com/treesitter-go/treesitter/internal/subtree"

var EditSubtree = st.EditSubtree
var LengthSaturatingSub = st.LengthSaturatingSub
