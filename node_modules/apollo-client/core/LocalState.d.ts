import { DocumentNode, FieldNode, ASTNode } from 'graphql';
import { ApolloCache } from 'apollo-cache';
import { FragmentMap } from 'apollo-utilities';
import { FetchResult } from 'apollo-link';
import ApolloClient from '../ApolloClient';
import { Resolvers, OperationVariables } from './types';
export declare type Resolver = (rootValue?: any, args?: any, context?: any, info?: {
    field: FieldNode;
    fragmentMap: FragmentMap;
}) => any;
export declare type VariableMap = {
    [name: string]: any;
};
export declare type FragmentMatcher = (rootValue: any, typeCondition: string, context: any) => boolean;
export declare type ExecContext = {
    fragmentMap: FragmentMap;
    context: any;
    variables: VariableMap;
    fragmentMatcher: FragmentMatcher;
    defaultOperationType: string;
    exportedVariables: Record<string, any>;
    onlyRunForcedResolvers: boolean;
};
export declare type LocalStateOptions<TCacheShape> = {
    cache: ApolloCache<TCacheShape>;
    client?: ApolloClient<TCacheShape>;
    resolvers?: Resolvers | Resolvers[];
    fragmentMatcher?: FragmentMatcher;
};
export declare class LocalState<TCacheShape> {
    private cache;
    private client;
    private resolvers?;
    private fragmentMatcher;
    constructor({ cache, client, resolvers, fragmentMatcher, }: LocalStateOptions<TCacheShape>);
    addResolvers(resolvers: Resolvers | Resolvers[]): void;
    setResolvers(resolvers: Resolvers | Resolvers[]): void;
    getResolvers(): Resolvers;
    runResolvers<TData>({ document, remoteResult, context, variables, onlyRunForcedResolvers, }: {
        document: DocumentNode | null;
        remoteResult: FetchResult<TData>;
        context?: Record<string, any>;
        variables?: Record<string, any>;
        onlyRunForcedResolvers?: boolean;
    }): Promise<FetchResult<TData>>;
    setFragmentMatcher(fragmentMatcher: FragmentMatcher): void;
    getFragmentMatcher(): FragmentMatcher;
    clientQuery(document: DocumentNode): DocumentNode | null;
    serverQuery(document: DocumentNode): DocumentNode | null;
    prepareContext(context?: {}): {
        cache: ApolloCache<TCacheShape>;
        getCacheKey: (obj: {
            __typename: string;
            id: string | number;
        }) => any;
    };
    addExportedVariables(document: DocumentNode, variables?: OperationVariables, context?: {}): Promise<{
        [x: string]: any;
    }>;
    shouldForceResolvers(document: ASTNode): boolean;
    private buildRootValueFromCache;
    private resolveDocument;
    private resolveSelectionSet;
    private resolveField;
    private resolveSubSelectedArray;
}
//# sourceMappingURL=LocalState.d.ts.map