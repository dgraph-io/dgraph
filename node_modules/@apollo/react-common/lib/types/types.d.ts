import ApolloClient, { ApolloQueryResult, ApolloError, FetchPolicy, WatchQueryFetchPolicy, ErrorPolicy, FetchMoreOptions, FetchMoreQueryOptions, PureQueryOptions, MutationUpdaterFn, NetworkStatus, ObservableQuery } from 'apollo-client';
import { DocumentNode, GraphQLError } from 'graphql';
export declare type OperationVariables = Record<string, any>;
export declare type Context = Record<string, any>;
export interface ExecutionResult<T = Record<string, any>> {
    data?: T;
    extensions?: Record<string, any>;
    errors?: GraphQLError[];
}
export interface BaseQueryOptions<TVariables = OperationVariables> {
    ssr?: boolean;
    variables?: TVariables;
    fetchPolicy?: WatchQueryFetchPolicy;
    errorPolicy?: ErrorPolicy;
    pollInterval?: number;
    client?: ApolloClient<any>;
    notifyOnNetworkStatusChange?: boolean;
    context?: Context;
    partialRefetch?: boolean;
    returnPartialData?: boolean;
}
export interface QueryFunctionOptions<TData = any, TVariables = OperationVariables> extends BaseQueryOptions<TVariables> {
    displayName?: string;
    skip?: boolean;
    onCompleted?: (data: TData) => void;
    onError?: (error: ApolloError) => void;
}
export declare type ObservableQueryFields<TData, TVariables> = Pick<ObservableQuery<TData, TVariables>, 'startPolling' | 'stopPolling' | 'subscribeToMore' | 'updateQuery' | 'refetch' | 'variables'> & {
    fetchMore: (<K extends keyof TVariables>(fetchMoreOptions: FetchMoreQueryOptions<TVariables, K> & FetchMoreOptions<TData, TVariables>) => Promise<ApolloQueryResult<TData>>) & (<TData2, TVariables2, K extends keyof TVariables2>(fetchMoreOptions: {
        query?: DocumentNode;
    } & FetchMoreQueryOptions<TVariables2, K> & FetchMoreOptions<TData2, TVariables2>) => Promise<ApolloQueryResult<TData2>>);
};
export interface QueryResult<TData = any, TVariables = OperationVariables> extends ObservableQueryFields<TData, TVariables> {
    client: ApolloClient<any>;
    data: TData | undefined;
    error?: ApolloError;
    loading: boolean;
    networkStatus: NetworkStatus;
    called: boolean;
}
export declare type RefetchQueriesFunction = (...args: any[]) => Array<string | PureQueryOptions>;
export interface BaseMutationOptions<TData = any, TVariables = OperationVariables> {
    variables?: TVariables;
    optimisticResponse?: TData | ((vars: TVariables) => TData);
    refetchQueries?: Array<string | PureQueryOptions> | RefetchQueriesFunction;
    awaitRefetchQueries?: boolean;
    errorPolicy?: ErrorPolicy;
    update?: MutationUpdaterFn<TData>;
    client?: ApolloClient<object>;
    notifyOnNetworkStatusChange?: boolean;
    context?: Context;
    onCompleted?: (data: TData) => void;
    onError?: (error: ApolloError) => void;
    fetchPolicy?: WatchQueryFetchPolicy;
    ignoreResults?: boolean;
}
export interface MutationFunctionOptions<TData = any, TVariables = OperationVariables> {
    variables?: TVariables;
    optimisticResponse?: TData | ((vars: TVariables | {}) => TData);
    refetchQueries?: Array<string | PureQueryOptions> | RefetchQueriesFunction;
    awaitRefetchQueries?: boolean;
    update?: MutationUpdaterFn<TData>;
    context?: Context;
    fetchPolicy?: WatchQueryFetchPolicy;
}
export interface MutationResult<TData = any> {
    data?: TData;
    error?: ApolloError;
    loading: boolean;
    called: boolean;
    client?: ApolloClient<object>;
}
export declare type MutationFetchResult<TData = Record<string, any>, C = Record<string, any>, E = Record<string, any>> = ExecutionResult<TData> & {
    extensions?: E;
    context?: C;
};
export declare type MutationFunction<TData = any, TVariables = OperationVariables> = (options?: MutationFunctionOptions<TData, TVariables>) => Promise<MutationFetchResult<TData>>;
export interface OnSubscriptionDataOptions<TData = any> {
    client: ApolloClient<object>;
    subscriptionData: SubscriptionResult<TData>;
}
export interface BaseSubscriptionOptions<TData = any, TVariables = OperationVariables> {
    variables?: TVariables;
    fetchPolicy?: FetchPolicy;
    shouldResubscribe?: boolean | ((options: BaseSubscriptionOptions<TData, TVariables>) => boolean);
    client?: ApolloClient<object>;
    skip?: boolean;
    onSubscriptionData?: (options: OnSubscriptionDataOptions<TData>) => any;
    onSubscriptionComplete?: () => void;
}
export interface SubscriptionResult<TData = any> {
    loading: boolean;
    data?: TData;
    error?: ApolloError;
}
