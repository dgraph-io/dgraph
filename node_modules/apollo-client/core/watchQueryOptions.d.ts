import { DocumentNode } from 'graphql';
import { FetchResult } from 'apollo-link';
import { DataProxy } from 'apollo-cache';
import { MutationQueryReducersMap } from './types';
import { PureQueryOptions, OperationVariables } from './types';
export declare type FetchPolicy = 'cache-first' | 'network-only' | 'cache-only' | 'no-cache' | 'standby';
export declare type WatchQueryFetchPolicy = FetchPolicy | 'cache-and-network';
export declare type ErrorPolicy = 'none' | 'ignore' | 'all';
export interface QueryBaseOptions<TVariables = OperationVariables> {
    query: DocumentNode;
    variables?: TVariables;
    errorPolicy?: ErrorPolicy;
    fetchResults?: boolean;
    metadata?: any;
    context?: any;
}
export interface QueryOptions<TVariables = OperationVariables> extends QueryBaseOptions<TVariables> {
    fetchPolicy?: FetchPolicy;
}
export interface ModifiableWatchQueryOptions<TVariables = OperationVariables> extends QueryBaseOptions<TVariables> {
    pollInterval?: number;
    notifyOnNetworkStatusChange?: boolean;
    returnPartialData?: boolean;
}
export interface WatchQueryOptions<TVariables = OperationVariables> extends QueryBaseOptions<TVariables>, ModifiableWatchQueryOptions<TVariables> {
    fetchPolicy?: WatchQueryFetchPolicy;
}
export interface FetchMoreQueryOptions<TVariables, K extends keyof TVariables> {
    query?: DocumentNode;
    variables?: Pick<TVariables, K>;
    context?: any;
}
export declare type UpdateQueryFn<TData = any, TSubscriptionVariables = OperationVariables, TSubscriptionData = TData> = (previousQueryResult: TData, options: {
    subscriptionData: {
        data: TSubscriptionData;
    };
    variables?: TSubscriptionVariables;
}) => TData;
export declare type SubscribeToMoreOptions<TData = any, TSubscriptionVariables = OperationVariables, TSubscriptionData = TData> = {
    document: DocumentNode;
    variables?: TSubscriptionVariables;
    updateQuery?: UpdateQueryFn<TData, TSubscriptionVariables, TSubscriptionData>;
    onError?: (error: Error) => void;
};
export interface SubscriptionOptions<TVariables = OperationVariables> {
    query: DocumentNode;
    variables?: TVariables;
    fetchPolicy?: FetchPolicy;
}
export declare type RefetchQueryDescription = Array<string | PureQueryOptions>;
export interface MutationBaseOptions<T = {
    [key: string]: any;
}, TVariables = OperationVariables> {
    optimisticResponse?: T | ((vars: TVariables) => T);
    updateQueries?: MutationQueryReducersMap<T>;
    refetchQueries?: ((result: FetchResult<T>) => RefetchQueryDescription) | RefetchQueryDescription;
    awaitRefetchQueries?: boolean;
    update?: MutationUpdaterFn<T>;
    errorPolicy?: ErrorPolicy;
    variables?: TVariables;
}
export interface MutationOptions<T = {
    [key: string]: any;
}, TVariables = OperationVariables> extends MutationBaseOptions<T, TVariables> {
    mutation: DocumentNode;
    context?: any;
    fetchPolicy?: FetchPolicy;
}
export declare type MutationUpdaterFn<T = {
    [key: string]: any;
}> = (proxy: DataProxy, mutationResult: FetchResult<T>) => void;
//# sourceMappingURL=watchQueryOptions.d.ts.map