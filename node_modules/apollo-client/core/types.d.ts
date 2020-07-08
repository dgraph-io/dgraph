import { FetchResult } from 'apollo-link';
import { DocumentNode, GraphQLError } from 'graphql';
import { QueryStoreValue } from '../data/queries';
import { NetworkStatus } from './networkStatus';
import { Resolver } from './LocalState';
export declare type QueryListener = (queryStoreValue: QueryStoreValue, newData?: any, forceResolvers?: boolean) => void;
export declare type OperationVariables = {
    [key: string]: any;
};
export declare type PureQueryOptions = {
    query: DocumentNode;
    variables?: {
        [key: string]: any;
    };
    context?: any;
};
export declare type ApolloQueryResult<T> = {
    data: T;
    errors?: ReadonlyArray<GraphQLError>;
    loading: boolean;
    networkStatus: NetworkStatus;
    stale: boolean;
};
export declare enum FetchType {
    normal = 1,
    refetch = 2,
    poll = 3
}
export declare type MutationQueryReducer<T> = (previousResult: Record<string, any>, options: {
    mutationResult: FetchResult<T>;
    queryName: string | undefined;
    queryVariables: Record<string, any>;
}) => Record<string, any>;
export declare type MutationQueryReducersMap<T = {
    [key: string]: any;
}> = {
    [queryName: string]: MutationQueryReducer<T>;
};
export interface Resolvers {
    [key: string]: {
        [field: string]: Resolver;
    };
}
//# sourceMappingURL=types.d.ts.map