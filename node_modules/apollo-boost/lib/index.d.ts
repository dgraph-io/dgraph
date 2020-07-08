export * from 'apollo-client';
export * from 'apollo-link';
export * from 'apollo-cache-inmemory';
import { Operation } from 'apollo-link';
import { HttpLink, UriFunction } from 'apollo-link-http';
import { ErrorLink } from 'apollo-link-error';
import { ApolloCache } from 'apollo-cache';
import { CacheResolverMap } from 'apollo-cache-inmemory';
import gql from 'graphql-tag';
import ApolloClient, { Resolvers, LocalStateFragmentMatcher } from 'apollo-client';
import { DocumentNode } from 'graphql';
export { gql, HttpLink };
declare type ClientStateConfig = {
    cache?: ApolloCache<any>;
    defaults?: Record<string, any>;
    resolvers?: Resolvers | Resolvers[];
    typeDefs?: string | string[] | DocumentNode | DocumentNode[];
    fragmentMatcher?: LocalStateFragmentMatcher;
};
export interface PresetConfig {
    request?: (operation: Operation) => Promise<void> | void;
    uri?: string | UriFunction;
    credentials?: string;
    headers?: any;
    fetch?: WindowOrWorkerGlobalScope['fetch'];
    fetchOptions?: HttpLink.Options;
    clientState?: ClientStateConfig;
    onError?: ErrorLink.ErrorHandler;
    cacheRedirects?: CacheResolverMap;
    cache?: ApolloCache<any>;
    name?: string;
    version?: string;
    resolvers?: Resolvers | Resolvers[];
    typeDefs?: string | string[] | DocumentNode | DocumentNode[];
    fragmentMatcher?: LocalStateFragmentMatcher;
    assumeImmutableResults?: boolean;
}
export default class DefaultClient<TCache> extends ApolloClient<TCache> {
    constructor(config?: PresetConfig);
}
//# sourceMappingURL=index.d.ts.map