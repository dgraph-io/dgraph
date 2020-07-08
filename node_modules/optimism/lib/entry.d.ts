import { OptimisticWrapOptions } from "./index";
declare type Value<T> = [] | [T] | [void, any];
export declare type AnyEntry = Entry<any, any>;
export declare class Entry<TArgs extends any[], TValue> {
    readonly fn: (...args: TArgs) => TValue;
    args: TArgs;
    static count: number;
    subscribe: OptimisticWrapOptions<TArgs>["subscribe"];
    unsubscribe?: () => any;
    reportOrphan?: (this: Entry<TArgs, TValue>) => any;
    readonly parents: Set<Entry<any, any>>;
    readonly childValues: Map<Entry<any, any>, Value<any>>;
    dirtyChildren: Set<AnyEntry> | null;
    dirty: boolean;
    recomputing: boolean;
    readonly value: Value<TValue>;
    constructor(fn: (...args: TArgs) => TValue, args: TArgs);
    recompute(): TValue;
    setDirty(): void;
    dispose(): void;
}
export {};
