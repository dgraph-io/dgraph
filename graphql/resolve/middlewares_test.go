/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package resolve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hypermodeinc/dgraph/v25/graphql/schema"
)

func TestQueryMiddlewares_Then_ExecutesMiddlewaresInOrder(t *testing.T) {
	array := make([]int, 0)
	addToArray := func(num int) {
		array = append(array, num)
	}
	m1 := QueryMiddleware(func(resolver QueryResolver) QueryResolver {
		return QueryResolverFunc(func(ctx context.Context, query schema.Query) *Resolved {
			addToArray(1)
			defer addToArray(5)
			return resolver.Resolve(ctx, query)
		})
	})
	m2 := QueryMiddleware(func(resolver QueryResolver) QueryResolver {
		return QueryResolverFunc(func(ctx context.Context, query schema.Query) *Resolved {
			addToArray(2)
			resolved := resolver.Resolve(ctx, query)
			addToArray(4)
			return resolved
		})
	})
	mws := QueryMiddlewares{m1, m2}

	resolver := mws.Then(QueryResolverFunc(func(ctx context.Context, query schema.Query) *Resolved {
		addToArray(3)
		return &Resolved{
			Field:      query,
			Extensions: &schema.Extensions{TouchedUids: 1},
		}
	}))
	resolved := resolver.Resolve(context.Background(), nil)

	require.Equal(t, &Resolved{Extensions: &schema.Extensions{TouchedUids: 1}}, resolved)
	require.Equal(t, []int{1, 2, 3, 4, 5}, array)
}

func TestMutationMiddlewares_Then_ExecutesMiddlewaresInOrder(t *testing.T) {
	array := make([]int, 0)
	addToArray := func(num int) {
		array = append(array, num)
	}
	m1 := MutationMiddleware(func(resolver MutationResolver) MutationResolver {
		return MutationResolverFunc(func(ctx context.Context, mutation schema.Mutation) (*Resolved, bool) {
			addToArray(1)
			defer addToArray(5)
			return resolver.Resolve(ctx, mutation)
		})
	})
	m2 := MutationMiddleware(func(resolver MutationResolver) MutationResolver {
		return MutationResolverFunc(func(ctx context.Context,
			mutation schema.Mutation) (*Resolved, bool) {
			addToArray(2)
			resolved, success := resolver.Resolve(ctx, mutation)
			addToArray(4)
			return resolved, success
		})
	})
	mws := MutationMiddlewares{m1, m2}

	resolver := mws.Then(MutationResolverFunc(func(ctx context.Context, mutation schema.Mutation) (*Resolved, bool) {
		addToArray(3)
		return &Resolved{
			Field:      mutation,
			Extensions: &schema.Extensions{TouchedUids: 1},
		}, true
	}))
	resolved, succeeded := resolver.Resolve(context.Background(), nil)

	require.True(t, succeeded)
	require.Equal(t, &Resolved{Extensions: &schema.Extensions{TouchedUids: 1}}, resolved)
	require.Equal(t, []int{1, 2, 3, 4, 5}, array)
}
