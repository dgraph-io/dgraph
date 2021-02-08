package admin

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

type namespaceInput struct {
	NamespaceId int
}

func resolveCreateNamespace(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	req, err := getNamespaceInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}
	if err = (&edgraph.Server{}).CreateNamespace(ctx, uint64(req.NamespaceId)); err != nil {
		return resolve.EmptyResult(m, err), false
	}
	return resolve.DataResult(
		m,
		map[string]interface{}{m.Name(): map[string]interface{}{
			"namespaceId": json.Number(strconv.Itoa(req.NamespaceId)),
			"message":     "Created namespace successfully",
		}},
		nil,
	), true
}

func resolveDeleteNamespace(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	req, err := getNamespaceInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}
	if err = (&edgraph.Server{}).DeleteNamespace(ctx, uint64(req.NamespaceId)); err != nil {
		return resolve.EmptyResult(m, err), false
	}
	return resolve.DataResult(
		m,
		map[string]interface{}{m.Name(): map[string]interface{}{
			"namespaceId": json.Number(strconv.Itoa(req.NamespaceId)),
			"message":     "Deleted namespace successfully",
		}},
		nil,
	), true
}

func getNamespaceInput(m schema.Mutation) (*namespaceInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input namespaceInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
