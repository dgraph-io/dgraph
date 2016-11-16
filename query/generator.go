package query

import (
	"context"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
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

	x.AssertTrue(len(sg.Result) == len(tokens))
	return sg, nil
}

func allOf(ctx context.Context, intersectUIDs *algo.UIDList,
	attr, terms string) (*algo.UIDList, error) {

	sg, err := fetchTermUids(ctx, intersectUIDs, attr, terms)
	if err != nil {
		return nil, err
	}
	return algo.IntersectLists(sg.Result), nil
}

func anyOf(ctx context.Context, intersectUIDs *algo.UIDList,
	attr, terms string) (*algo.UIDList, error) {

	sg, err := fetchTermUids(ctx, intersectUIDs, attr, terms)
	if err != nil {
		return nil, err
	}
	return algo.MergeLists(sg.Result), nil
}

func near(ctx context.Context, intersectUIDs *algo.UIDList, attr, point, dist string) (*algo.UIDList, error) {
	maxD, err := strconv.ParseFloat(dist, 64)
	if err != nil {
		return nil, err
	}
	var g types.Geo
	geoD := strings.Replace(point, "'", "\"", -1)
	err = g.UnmarshalText([]byte(geoD))
	if err != nil {
		return nil, err
	}
	gb, err := g.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return filterGeo(ctx, attr, geo.QueryTypeNear, gb, maxD, intersectUIDs)
}

func within(ctx context.Context, intersectUIDs *algo.UIDList, attr, region string) (*algo.UIDList, error) {
	var g types.Geo
	geoD := strings.Replace(region, "'", "\"", -1)
	err := g.UnmarshalText([]byte(geoD))
	if err != nil {
		return nil, x.Wrapf(err, "Cannot decode given geoJson input")
	}
	gb, err := g.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return filterGeo(ctx, attr, geo.QueryTypeWithin, gb, 0, intersectUIDs)
}

func contains(ctx context.Context, intersectUIDs *algo.UIDList, attr, region string) (*algo.UIDList, error) {
	var g types.Geo
	geoD := strings.Replace(region, "'", "\"", -1)
	err := g.UnmarshalText([]byte(geoD))
	if err != nil {
		return nil, err
	}
	gb, err := g.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return filterGeo(ctx, attr, geo.QueryTypeContains, gb, 0, intersectUIDs)
}

func intersects(ctx context.Context, intersectUIDs *algo.UIDList, attr, region string) (*algo.UIDList, error) {
	var g types.Geo
	geoD := strings.Replace(region, "'", "\"", -1)
	err := g.UnmarshalText([]byte(geoD))
	if err != nil {
		return nil, x.Wrapf(err, "Cannot decode given geoJson input")
	}
	gb, err := g.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return filterGeo(ctx, attr, geo.QueryTypeIntersects, gb, 0, intersectUIDs)
}
