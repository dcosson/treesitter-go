package treesitter

// tree.go re-exports Tree, Node, and helpers from internal/tree.

import itree "github.com/treesitter-go/treesitter/internal/tree"

// Tree represents a complete parse tree.
type Tree = itree.Tree

// Node is a lightweight value type representing a node in the parse tree.
type Node = itree.Node

// NewTree creates a new Tree from a root subtree and associated metadata.
var NewTree = itree.NewTree

// AdvancePosition moves position past a subtree (its padding + size).
var AdvancePosition = itree.AdvancePosition
