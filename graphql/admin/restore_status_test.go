package admin

import (
	"context"
	"encoding/json"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestRestoreStatus(t *testing.T) {
	gqlSchema := test.LoadSchema(t, graphqlAdminSchema)
	gqlQuery := `query restoreStatus($restoreId: Int!) {
					restoreStatus(restoreId: $restoreId) {
						status
						errors
					}
				}`
	variables := `{"restoreId": 2 }`
	vars := make(map[string]interface{})
	d := json.NewDecoder(strings.NewReader(variables))
	d.UseNumber()
	err := d.Decode(&vars)
	require.NoError(t, err)

	op, err:= gqlSchema.Operation(
		&schema.Request{
			Query: gqlQuery,
			Variables: vars,
		})
	require.NoError(t, err)
	GQLQuery:= test.GetQuery(t, op)
	resolved:= resolveRestoreStatus(context.Background(), GQLQuery)
	require.NoError(t, resolved.Err)
}