/* necessary for backward compat */
export * from 'apollo-client';
export * from 'apollo-link';
export * from 'apollo-cache-inmemory';

import { Operation, ApolloLink, Observable } from 'apollo-link';
import { HttpLink, UriFunction } from 'apollo-link-http';
import { onError, ErrorLink } from 'apollo-link-error';
import { ApolloCache } from 'apollo-cache';
import { InMemoryCache, CacheResolverMap } from 'apollo-cache-inmemory';
import gql from 'graphql-tag';
import ApolloClient, {
  Resolvers,
  LocalStateFragmentMatcher,
} from 'apollo-client';
import { DocumentNode } from 'graphql';
import { invariant } from 'ts-invariant';

export { gql, HttpLink };

type ClientStateConfig = {
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

// Yes, these are the exact same as the `PresetConfig` interface. We're
// defining these again so they can be used to verify that valid config
// options are being used in the `DefaultClient` constructor, for clients
// that aren't using Typescript. This duplication is unfortunate, and at
// some point can likely be adjusted so these items are inferred from
// the `PresetConfig` interface using a Typescript transform at compilation
// time. Unfortunately, TS transforms with rollup don't appear to be quite
// working properly, so this will have to be re-visited at some point.
// For now, when updating the properties of the `PresetConfig` interface,
// please also update this constant.
const PRESET_CONFIG_KEYS = [
  'request',
  'uri',
  'credentials',
  'headers',
  'fetch',
  'fetchOptions',
  'clientState',
  'onError',
  'cacheRedirects',
  'cache',
  'name',
  'version',
  'resolvers',
  'typeDefs',
  'fragmentMatcher',
];

export default class DefaultClient<TCache> extends ApolloClient<TCache> {
  constructor(config: PresetConfig = {}) {
    if (config) {
      const diff = Object.keys(config).filter(
        key => PRESET_CONFIG_KEYS.indexOf(key) === -1,
      );

      if (diff.length > 0) {
        invariant.warn(
          'ApolloBoost was initialized with unsupported options: ' +
            `${diff.join(' ')}`,
        );
      }
    }

    const {
      request,
      uri,
      credentials,
      headers,
      fetch,
      fetchOptions,
      clientState,
      cacheRedirects,
      onError: errorCallback,
      name,
      version,
      resolvers,
      typeDefs,
      fragmentMatcher,
    } = config;

    let { cache } = config;

    invariant(
      !cache || !cacheRedirects,
      'Incompatible cache configuration. When not providing `cache`, ' +
        'configure the provided instance with `cacheRedirects` instead.',
    );

    if (!cache) {
      cache = cacheRedirects
        ? new InMemoryCache({ cacheRedirects })
        : new InMemoryCache();
    }

    const errorLink = errorCallback
      ? onError(errorCallback)
      : onError(({ graphQLErrors, networkError }) => {
          if (graphQLErrors) {
            graphQLErrors.forEach(({ message, locations, path }) =>
              // tslint:disable-next-line
              invariant.warn(
                `[GraphQL error]: Message: ${message}, Location: ` +
                  `${locations}, Path: ${path}`,
              ),
            );
          }
          if (networkError) {
            // tslint:disable-next-line
            invariant.warn(`[Network error]: ${networkError}`);
          }
        });

    const requestHandler = request
      ? new ApolloLink(
          (operation, forward) =>
            new Observable(observer => {
              let handle: any;
              Promise.resolve(operation)
                .then(oper => request(oper))
                .then(() => {
                  handle = forward(operation).subscribe({
                    next: observer.next.bind(observer),
                    error: observer.error.bind(observer),
                    complete: observer.complete.bind(observer),
                  });
                })
                .catch(observer.error.bind(observer));

              return () => {
                if (handle) {
                  handle.unsubscribe();
                }
              };
            }),
        )
      : false;

    const httpLink = new HttpLink({
      uri: uri || '/graphql',
      fetch,
      fetchOptions: fetchOptions || {},
      credentials: credentials || 'same-origin',
      headers: headers || {},
    });

    const link = ApolloLink.from([errorLink, requestHandler, httpLink].filter(
      x => !!x,
    ) as ApolloLink[]);

    let activeResolvers = resolvers;
    let activeTypeDefs = typeDefs;
    let activeFragmentMatcher = fragmentMatcher;
    if (clientState) {
      if (clientState.defaults) {
        cache.writeData({
          data: clientState.defaults,
        });
      }
      activeResolvers = clientState.resolvers;
      activeTypeDefs = clientState.typeDefs;
      activeFragmentMatcher = clientState.fragmentMatcher;
    }

    // super hacky, we will fix the types eventually
    super({
      cache,
      link,
      name,
      version,
      resolvers: activeResolvers,
      typeDefs: activeTypeDefs,
      fragmentMatcher: activeFragmentMatcher,
    } as any);
  }
}
