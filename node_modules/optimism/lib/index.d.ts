import { KeyTrie } from "./key-trie";
export { bindContext, noContext, setTimeout, asyncFromGen, } from "./context";
export declare type TCacheKey = any;
export declare function defaultMakeCacheKey(...args: any[]): any;
export { KeyTrie };
export declare type OptimisticWrapperFunction<TArgs extends any[], TResult> = ((...args: TArgs) => TResult) & {
    dirty: (...args: TArgs) => void;
};
export declare type OptimisticWrapOptions<TArgs extends any[]> = {
    max?: number;
    disposable?: boolean;
    makeCacheKey?: (...args: TArgs) => TCacheKey;
    subscribe?: (...args: TArgs) => (() => any) | undefined;
};
export declare function wrap<TArgs extends any[], TResult>(originalFunction: (...args: TArgs) => TResult, options?: OptimisticWrapOptions<TArgs>): OptimisticWrapperFunction<TArgs, TResult>;
