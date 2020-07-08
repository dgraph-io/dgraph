import { ExecutionResult, DocumentNode } from 'graphql';
import { ApolloCache, DataProxy } from 'apollo-cache';
import { QueryStoreValue } from '../data/queries';
import { MutationQueryReducer } from '../core/types';
export declare type QueryWithUpdater = {
    updater: MutationQueryReducer<Object>;
    query: QueryStoreValue;
};
export interface DataWrite {
    rootId: string;
    result: any;
    document: DocumentNode;
    operationName: string | null;
    variables: Object;
}
export declare class DataStore<TSerialized> {
    private cache;
    constructor(initialCache: ApolloCache<TSerialized>);
    getCache(): ApolloCache<TSerialized>;
    markQueryResult(result: ExecutionResult, document: DocumentNode, variables: any, fetchMoreForQueryId: string | undefined, ignoreErrors?: boolean): void;
    markSubscriptionResult(result: ExecutionResult, document: DocumentNode, variables: any): void;
    markMutationInit(mutation: {
        mutationId: string;
        document: DocumentNode;
        variables: any;
        updateQueries: {
            [queryId: string]: QueryWithUpdater;
        };
        update: ((proxy: DataProxy, mutationResult: Object) => void) | undefined;
        optimisticResponse: Object | Function | undefined;
    }): void;
    markMutationResult(mutation: {
        mutationId: string;
        result: ExecutionResult;
        document: DocumentNode;
        variables: any;
        updateQueries: {
            [queryId: string]: QueryWithUpdater;
        };
        update: ((proxy: DataProxy, mutationResult: Object) => void) | undefined;
    }): void;
    markMutationComplete({ mutationId, optimisticResponse, }: {
        mutationId: string;
        optimisticResponse?: any;
    }): void;
    markUpdateQueryResult(document: DocumentNode, variables: any, newResult: any): void;
    reset(): Promise<void>;
}
//# sourceMappingURL=store.d.ts.map