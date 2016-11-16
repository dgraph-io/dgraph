package query

import (
	"context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
)

func fetchTermUids(ctx context.Context, intersectUIDs *algo.UIDList,
	attr, terms string) (*SubGraph, error) {

	// Tokenize the terms.
	tokenizer, err := tok.NewTokenizer([]byte(terms))
	if err != nil {
		return nil, x.Errorf("Could not create tokenizer: %v", terms)
	}
	defer tokenizer.Destroy()
	tokens := tokenizer.Tokens()

	sg := &SubGraph{Attr: attr}
	sgChan := make(chan error, 1)
	taskQuery := createTaskQuery(sg, nil, tokens, intersectUIDs)
	go ProcessGraph(ctx, sg, taskQuery, sgChan)
	select {
	case <-ctx.Done():
		return nil, x.Wrap(ctx.Err())
	case err = <-sgChan:
		if err != nil {
			return nil, err
		}
	}

	x.AssertTrue(len(sg.UIDMatrix()) == len(tokens))
	return sg, nil
}

func allOf(ctx context.Context, intersectUIDs *algo.UIDList,
	attr, terms string) (*algo.UIDList, error) {

	sg, err := fetchTermUids(ctx, intersectUIDs, attr, terms)
	if err != nil {
		return nil, err
	}
	return algo.IntersectLists(sg.UIDMatrix()), nil
}

func anyOf(ctx context.Context, intersectUIDs *algo.UIDList,
	attr, terms string) (*algo.UIDList, error) {

	sg, err := fetchTermUids(ctx, intersectUIDs, attr, terms)
	if err != nil {
		return nil, err
	}
	return algo.MergeLists(sg.UIDMatrix()), nil
}
