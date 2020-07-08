export declare const parentEntrySlot: {
    readonly id: string;
    hasValue(): boolean;
    getValue(): import("./entry").Entry<any, any> | undefined;
    withValue<TResult, TArgs extends any[], TThis = any>(value: import("./entry").Entry<any, any>, callback: (this: TThis, ...args: TArgs) => TResult, args?: TArgs | undefined, thisArg?: TThis | undefined): TResult;
};
export { bind as bindContext, noContext, setTimeout, asyncFromGen, } from "@wry/context";
