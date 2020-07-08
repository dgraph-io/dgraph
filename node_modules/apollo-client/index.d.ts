export { ObservableQuery, FetchMoreOptions, UpdateQueryOptions, ApolloCurrentResult, ApolloCurrentQueryResult, } from './core/ObservableQuery';
export { QueryBaseOptions, QueryOptions, WatchQueryOptions, MutationOptions, SubscriptionOptions, FetchPolicy, WatchQueryFetchPolicy, ErrorPolicy, FetchMoreQueryOptions, SubscribeToMoreOptions, MutationUpdaterFn, } from './core/watchQueryOptions';
export { NetworkStatus } from './core/networkStatus';
export * from './core/types';
export { Resolver, FragmentMatcher as LocalStateFragmentMatcher, } from './core/LocalState';
export { isApolloError, ApolloError } from './errors/ApolloError';
import ApolloClient, { ApolloClientOptions, DefaultOptions } from './ApolloClient';
export { ApolloClientOptions, DefaultOptions };
export { ApolloClient };
export default ApolloClient;
//# sourceMappingURL=index.d.ts.map