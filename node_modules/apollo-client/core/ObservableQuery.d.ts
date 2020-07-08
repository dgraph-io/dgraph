import { GraphQLError } from 'graphql';
import { NetworkStatus } from './networkStatus';
import { Observable } from '../util/Observable';
import { ApolloError } from '../errors/ApolloError';
import { QueryManager } from './QueryManager';
import { ApolloQueryResult, OperationVariables } from './types';
import { WatchQueryOptions, FetchMoreQueryOptions, SubscribeToMoreOptions, ErrorPolicy } from './watchQueryOptions';
import { QueryStoreValue } from '../data/queries';
export declare type ApolloCurrentResult<T> = {
    data: T | {};
    errors?: ReadonlyArray<GraphQLError>;
    loading: boolean;
    networkStatus: NetworkStatus;
    error?: ApolloError;
    partial?: boolean;
};
export declare type ApolloCurrentQueryResult<T> = {
    data: T | undefined;
    errors?: ReadonlyArray<GraphQLError>;
    loading: boolean;
    networkStatus: NetworkStatus;
    error?: ApolloError;
    partial?: boolean;
    stale?: boolean;
};
export interface FetchMoreOptions<TData = any, TVariables = OperationVariables> {
    updateQuery: (previousQueryResult: TData, options: {
        fetchMoreResult?: TData;
        variables?: TVariables;
    }) => TData;
}
export interface UpdateQueryOptions<TVariables> {
    variables?: TVariables;
}
export declare const hasError: (storeValue: QueryStoreValue, policy?: ErrorPolicy) => boolean | Error;
export declare class ObservableQuery<TData = any, TVariables = OperationVariables> extends Observable<ApolloQueryResult<TData>> {
    options: WatchQueryOptions<TVariables>;
    readonly queryId: string;
    readonly queryName?: string;
    variables: TVariables;
    private shouldSubscribe;
    private isTornDown;
    private queryManager;
    private observers;
    private subscriptions;
    private lastResult;
    private lastResultSnapshot;
    private lastError;
    constructor({ queryManager, options, shouldSubscribe, }: {
        queryManager: QueryManager<any>;
        options: WatchQueryOptions<TVariables>;
        shouldSubscribe?: boolean;
    });
    result(): Promise<ApolloQueryResult<TData>>;
    currentResult(): ApolloCurrentResult<TData>;
    getCurrentResult(): ApolloCurrentQueryResult<TData>;
    isDifferentFromLastResult(newResult: ApolloQueryResult<TData>): boolean;
    getLastResult(): ApolloQueryResult<TData>;
    getLastError(): ApolloError;
    resetLastResults(): void;
    resetQueryStoreErrors(): void;
    refetch(variables?: TVariables): Promise<ApolloQueryResult<TData>>;
    fetchMore<K extends keyof TVariables>(fetchMoreOptions: FetchMoreQueryOptions<TVariables, K> & FetchMoreOptions<TData, TVariables>): Promise<ApolloQueryResult<TData>>;
    subscribeToMore<TSubscriptionData = TData, TSubscriptionVariables = TVariables>(options: SubscribeToMoreOptions<TData, TSubscriptionVariables, TSubscriptionData>): () => void;
    setOptions(opts: WatchQueryOptions): Promise<ApolloQueryResult<TData> | void>;
    setVariables(variables: TVariables, tryFetch?: boolean, fetchResults?: boolean): Promise<ApolloQueryResult<TData> | void>;
    updateQuery<TVars = TVariables>(mapFn: (previousQueryResult: TData, options: UpdateQueryOptions<TVars>) => TData): void;
    stopPolling(): void;
    startPolling(pollInterval: number): void;
    private updateLastResult;
    private onSubscribe;
    private setUpQuery;
    private tearDownQuery;
}
//# sourceMappingURL=ObservableQuery.d.ts.map