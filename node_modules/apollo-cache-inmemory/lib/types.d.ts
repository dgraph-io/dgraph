import { DocumentNode } from 'graphql';
import { FragmentMatcher } from './readFromStore';
import { Transaction } from 'apollo-cache';
import { IdValue, StoreValue } from 'apollo-utilities';
export interface IdGetterObj extends Object {
    __typename?: string;
    id?: string;
}
export declare type IdGetter = (value: IdGetterObj) => string | null | undefined;
export interface NormalizedCache {
    get(dataId: string): StoreObject;
    set(dataId: string, value: StoreObject): void;
    delete(dataId: string): void;
    clear(): void;
    toObject(): NormalizedCacheObject;
    replace(newData: NormalizedCacheObject): void;
}
export interface NormalizedCacheObject {
    [dataId: string]: StoreObject | undefined;
}
export interface StoreObject {
    __typename?: string;
    [storeFieldKey: string]: StoreValue;
}
export declare type OptimisticStoreItem = {
    id: string;
    data: NormalizedCacheObject;
    transaction: Transaction<NormalizedCacheObject>;
};
export declare type ReadQueryOptions = {
    store: NormalizedCache;
    query: DocumentNode;
    fragmentMatcherFunction?: FragmentMatcher;
    variables?: Object;
    previousResult?: any;
    rootId?: string;
    config?: ApolloReducerConfig;
};
export declare type DiffQueryAgainstStoreOptions = ReadQueryOptions & {
    returnPartialData?: boolean;
};
export declare type ApolloReducerConfig = {
    dataIdFromObject?: IdGetter;
    fragmentMatcher?: FragmentMatcherInterface;
    addTypename?: boolean;
    cacheRedirects?: CacheResolverMap;
};
export declare type ReadStoreContext = {
    readonly store: NormalizedCache;
    readonly cacheRedirects: CacheResolverMap;
    readonly dataIdFromObject?: IdGetter;
};
export interface FragmentMatcherInterface {
    match(idValue: IdValue, typeCondition: string, context: ReadStoreContext): boolean | 'heuristic';
}
export declare type PossibleTypesMap = {
    [key: string]: string[];
};
export declare type IntrospectionResultData = {
    __schema: {
        types: {
            kind: string;
            name: string;
            possibleTypes: {
                name: string;
            }[];
        }[];
    };
};
export declare type CacheResolver = (rootValue: any, args: {
    [argName: string]: any;
}, context: any) => any;
export declare type CacheResolverMap = {
    [typeName: string]: {
        [fieldName: string]: CacheResolver;
    };
};
export declare type CustomResolver = CacheResolver;
export declare type CustomResolverMap = CacheResolverMap;
//# sourceMappingURL=types.d.ts.map