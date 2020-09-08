package admin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/stretchr/testify/require"
)

func TestRestoreStatus(t *testing.T) {
	gqlSchema := test.LoadSchema(t, graphqlAdminSchema)
	Query := `query restoreStatus($restoreId: Int!) {
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

	op, err := gqlSchema.Operation(
		&schema.Request{
			Query:     Query,
			Variables: vars,
		})
	require.NoError(t, err)
	gqlQuery := test.GetQuery(t, op)
	v, err := getRestoreStatusInput(gqlQuery)
	require.NoError(t, err)
	require.IsType(t, int64(2), v, nil)
}
