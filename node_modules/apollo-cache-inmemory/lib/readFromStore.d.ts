import { IdValue } from 'apollo-utilities';
import { Cache } from 'apollo-cache';
import { ReadStoreContext, DiffQueryAgainstStoreOptions, ReadQueryOptions, StoreObject } from './types';
import { KeyTrie } from 'optimism';
export declare type VariableMap = {
    [name: string]: any;
};
export declare type FragmentMatcher = (rootValue: any, typeCondition: string, context: ReadStoreContext) => boolean | 'heuristic';
export declare type ExecResultMissingField = {
    object: StoreObject;
    fieldName: string;
    tolerable: boolean;
};
export declare type ExecResult<R = any> = {
    result: R;
    missing?: ExecResultMissingField[];
};
export interface StoreReaderConfig {
    cacheKeyRoot?: KeyTrie<object>;
    freezeResults?: boolean;
}
export declare class StoreReader {
    private freezeResults;
    constructor({ cacheKeyRoot, freezeResults, }?: StoreReaderConfig);
    readQueryFromStore<QueryType>(options: ReadQueryOptions): QueryType | undefined;
    diffQueryAgainstStore<T>({ store, query, variables, previousResult, returnPartialData, rootId, fragmentMatcherFunction, config, }: DiffQueryAgainstStoreOptions): Cache.DiffResult<T>;
    private executeStoreQuery;
    private executeSelectionSet;
    private executeField;
    private combineExecResults;
    private executeSubSelectedArray;
}
export declare function assertIdValue(idValue: IdValue): void;
//# sourceMappingURL=readFromStore.d.ts.map