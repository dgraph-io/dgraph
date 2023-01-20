/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

// QueryMiddleware represents a middleware for queries
type QueryMiddleware func(resolver QueryResolver) QueryResolver

// MutationMiddleware represents a middleware for mutations
type MutationMiddleware func(resolver MutationResolver) MutationResolver

// QueryMiddlewares represents a list of middlewares for queries, that get applied in the order
// they are present in the list.
// Inspired from: https://github.com/justinas/alice
type QueryMiddlewares []QueryMiddleware

// MutationMiddlewares represents a list of middlewares for mutations, that get applied in the order
// they are present in the list.
// Inspired from: https://github.com/justinas/alice
type MutationMiddlewares []MutationMiddleware

// Then chains the middlewares and returns the final QueryResolver.
//
//	QueryMiddlewares{m1, m2, m3}.Then(r)
//
// is equivalent to:
//
//	m1(m2(m3(r)))
//
// When the request comes in, it will be passed to m1, then m2, then m3
// and finally, the given resolverFunc
// (assuming every middleware calls the following one).
//
// A chain can be safely reused by calling Then() several times.
//
//	commonMiddlewares := QueryMiddlewares{authMiddleware, loggingMiddleware}
//	healthResolver = commonMiddlewares.Then(resolveHealth)
//	stateResolver = commonMiddlewares.Then(resolveState)
//
// Note that middlewares are called on every call to Then()
// and thus several instances of the same middleware will be created
// when a chain is reused in this way.
// For proper middleware, this should cause no problems.
//
// Then() treats nil as a QueryResolverFunc that resolves to &Resolved{Field: query}
func (mws QueryMiddlewares) Then(resolver QueryResolver) QueryResolver {
	if len(mws) == 0 {
		return resolver
	}
	if resolver == nil {
		resolver = QueryResolverFunc(func(ctx context.Context, query schema.Query) *Resolved {
			return &Resolved{Field: query}
		})
	}
	for i := len(mws) - 1; i >= 0; i-- {
		resolver = mws[i](resolver)
	}
	return resolver
}

// Then chains the middlewares and returns the final MutationResolver.
//
//	MutationMiddlewares{m1, m2, m3}.Then(r)
//
// is equivalent to:
//
//	m1(m2(m3(r)))
//
// When the request comes in, it will be passed to m1, then m2, then m3
// and finally, the given resolverFunc
// (assuming every middleware calls the following one).
//
// A chain can be safely reused by calling Then() several times.
//
//	commonMiddlewares := MutationMiddlewares{authMiddleware, loggingMiddleware}
//	backupResolver = commonMiddlewares.Then(resolveBackup)
//	configResolver = commonMiddlewares.Then(resolveConfig)
//
// Note that middlewares are called on every call to Then()
// and thus several instances of the same middleware will be created
// when a chain is reused in this way.
// For proper middleware, this should cause no problems.
//
// Then() treats nil as a MutationResolverFunc that resolves to (&Resolved{Field: mutation}, true)
func (mws MutationMiddlewares) Then(resolver MutationResolver) MutationResolver {
	if len(mws) == 0 {
		return resolver
	}
	if resolver == nil {
		resolver = MutationResolverFunc(func(ctx context.Context,
			mutation schema.Mutation) (*Resolved, bool) {
			return &Resolved{Field: mutation}, true
		})
	}
	for i := len(mws) - 1; i >= 0; i-- {
		resolver = mws[i](resolver)
	}
	return resolver
}

// resolveGuardianOfTheGalaxyAuth returns a Resolved with error if the context doesn't contain any
// Guardian of Galaxy auth, otherwise it returns nil
func resolveGuardianOfTheGalaxyAuth(ctx context.Context, f schema.Field) *Resolved {
	if err := edgraph.AuthGuardianOfTheGalaxy(ctx); err != nil {
		return EmptyResult(f, err)
	}
	return nil
}

// resolveGuardianAuth returns a Resolved with error if the context doesn't contain any Guardian auth,
// otherwise it returns nil
func resolveGuardianAuth(ctx context.Context, f schema.Field) *Resolved {
	if err := edgraph.AuthorizeGuardians(ctx); err != nil {
		return EmptyResult(f, err)
	}
	return nil
}

func resolveIpWhitelisting(ctx context.Context, f schema.Field) *Resolved {
	if _, err := x.HasWhitelistedIP(ctx); err != nil {
		return EmptyResult(f, err)
	}
	return nil
}

// GuardianOfTheGalaxyAuthMW4Query blocks the resolution of resolverFunc if there is no Guardian
// of Galaxy auth present in context, otherwise it lets the resolverFunc resolve the query.
func GuardianOfTheGalaxyAuthMW4Query(resolver QueryResolver) QueryResolver {
	return QueryResolverFunc(func(ctx context.Context, query schema.Query) *Resolved {
		if resolved := resolveGuardianOfTheGalaxyAuth(ctx, query); resolved != nil {
			return resolved
		}
		return resolver.Resolve(ctx, query)
	})
}

// GuardianAuthMW4Query blocks the resolution of resolverFunc if there is no Guardian auth present
// in context, otherwise it lets the resolverFunc resolve the query.
func GuardianAuthMW4Query(resolver QueryResolver) QueryResolver {
	return QueryResolverFunc(func(ctx context.Context, query schema.Query) *Resolved {
		if resolved := resolveGuardianAuth(ctx, query); resolved != nil {
			return resolved
		}
		return resolver.Resolve(ctx, query)
	})
}

func IpWhitelistingMW4Query(resolver QueryResolver) QueryResolver {
	return QueryResolverFunc(func(ctx context.Context, query schema.Query) *Resolved {
		if resolved := resolveIpWhitelisting(ctx, query); resolved != nil {
			return resolved
		}
		return resolver.Resolve(ctx, query)
	})
}

func LoggingMWQuery(resolver QueryResolver) QueryResolver {
	return QueryResolverFunc(func(ctx context.Context, query schema.Query) *Resolved {
		glog.Infof("GraphQL admin query. Name =  %v", query.Name())
		return resolver.Resolve(ctx, query)
	})
}

// GuardianOfTheGalaxyAuthMW4Mutation blocks the resolution of resolverFunc if there is no Guardian
// of Galaxy auth present in context, otherwise it lets the resolverFunc resolve the mutation.
func GuardianOfTheGalaxyAuthMW4Mutation(resolver MutationResolver) MutationResolver {
	return MutationResolverFunc(func(ctx context.Context, mutation schema.Mutation) (*Resolved, bool) {
		if resolved := resolveGuardianOfTheGalaxyAuth(ctx, mutation); resolved != nil {
			return resolved, false
		}
		return resolver.Resolve(ctx, mutation)
	})
}

// GuardianAuthMW4Mutation blocks the resolution of resolverFunc if there is no Guardian auth
// present in context, otherwise it lets the resolverFunc resolve the mutation.
func GuardianAuthMW4Mutation(resolver MutationResolver) MutationResolver {
	return MutationResolverFunc(func(ctx context.Context, mutation schema.Mutation) (*Resolved, bool) {
		if resolved := resolveGuardianAuth(ctx, mutation); resolved != nil {
			return resolved, false
		}
		return resolver.Resolve(ctx, mutation)
	})
}

func IpWhitelistingMW4Mutation(resolver MutationResolver) MutationResolver {
	return MutationResolverFunc(func(ctx context.Context, mutation schema.Mutation) (*Resolved,
		bool) {
		if resolved := resolveIpWhitelisting(ctx, mutation); resolved != nil {
			return resolved, false
		}
		return resolver.Resolve(ctx, mutation)
	})
}

func LoggingMWMutation(resolver MutationResolver) MutationResolver {
	return MutationResolverFunc(func(ctx context.Context, mutation schema.Mutation) (*Resolved,
		bool) {
		glog.Infof("GraphQL admin mutation. Name =  %v", mutation.Name())
		return resolver.Resolve(ctx, mutation)
	})
}

func AclOnlyMW4Mutation(resolver MutationResolver) MutationResolver {
	return MutationResolverFunc(func(ctx context.Context, mutation schema.Mutation) (*Resolved,
		bool) {
		if !x.WorkerConfig.AclEnabled {
			return EmptyResult(mutation, errors.New("Enable ACL to use this mutation")), false
		}
		return resolver.Resolve(ctx, mutation)
	})
}
