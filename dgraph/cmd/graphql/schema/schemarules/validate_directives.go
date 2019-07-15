package schema

import (
	. "github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

func init() {
	AddSchRule("ValidateDirectives", func(sch *ast.SchemaDocument) *gqlerror.Error {
		// Leaving it for now. Will add this once we have support for directives.
		return nil
	})
}
