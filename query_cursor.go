package treesitter

// query_cursor.go re-exports QueryCursor from internal/query.

import iq "github.com/treesitter-go/treesitter/internal/query"

type QueryCursor = iq.QueryCursor

var NewQueryCursor = iq.NewQueryCursor
