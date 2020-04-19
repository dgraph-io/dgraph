/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package resolve

import (
	"context"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

var (
	CommonMutationMiddlewares = MutationMiddlewares{mutationAuthMiddleware}
)

// MutationMiddleware represents a middleware for mutations
type MutationMiddleware func(resolverFunc MutationResolverFunc) MutationResolverFunc

// MutationMiddlewares represents a list of middlewares for mutations, that get applied in the order
// they are present in the list.
// Inspired from: https://github.com/justinas/alice
type MutationMiddlewares []MutationMiddleware

// Then chains the middlewares and returns the final MutationResolverFunc.
//     MutationMiddlewares{m1, m2, m3}.Then(r)
// is equivalent to:
//     m1(m2(m3(r)))
// When the request comes in, it will be passed to m1, then m2, then m3
// and finally, the given resolverFunc
// (assuming every middleware calls the following one).
//
// A chain can be safely reused by calling Then() several times.
//     commonMiddlewares := MutationMiddlewares{authMiddleware, loggingMiddleware}
//     backupResolver = commonMiddlewares.Then(resolveBackup)
//     configResolver = commonMiddlewares.Then(resolveConfig)
// Note that constructors are called on every call to Then()
// and thus several instances of the same middleware will be created
// when a chain is reused in this way.
// For proper middleware, this should cause no problems.
//
// Then() treats nil as a MutationResolverFunc that resolves to (&Resolved{Field: mutation}, true)
func (mms MutationMiddlewares) Then(resolverFunc MutationResolverFunc) MutationResolverFunc {
	if resolverFunc == nil {
		resolverFunc = func(ctx context.Context, mutation schema.Mutation) (*Resolved, bool) {
			return &Resolved{Field: mutation}, true
		}
	}
	for i := len(mms) - 1; i >= 0; i-- {
		resolverFunc = mms[i](resolverFunc)
	}
	return resolverFunc
}

// resolveAuth returns a Resolved with error if the context doesn't contain any Guardian auth,
// otherwise it returns nil
func resolveAuth(ctx context.Context, f schema.Field) *Resolved {
	if err := edgraph.AuthorizeGuardians(ctx); err != nil {
		return &Resolved{
			Field: f,
			Err:   schema.AsGQLErrors(err),
		}
	}
	return nil
}

// mutationAuthMiddleware blocks the resolution of resolverFunc if there is no Guardian auth
// present in context, otherwise it lets the resolverFunc resolve the mutation.
func mutationAuthMiddleware(resolverFunc MutationResolverFunc) MutationResolverFunc {
	return func(ctx context.Context, mutation schema.Mutation) (*Resolved, bool) {
		if resolved := resolveAuth(ctx, mutation); resolved != nil {
			return resolved, false
		}
		return resolverFunc.Resolve(ctx, mutation)
	}
}
