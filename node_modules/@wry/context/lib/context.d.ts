/// <reference types="node" />
import { Slot } from "./slot";
export { Slot };
export declare const bind: <TArgs extends any[], TResult>(callback: (...args: TArgs) => TResult) => (...args: TArgs) => TResult, noContext: <TResult, TArgs extends any[], TThis = any>(callback: (this: TThis, ...args: TArgs) => TResult, args?: TArgs | undefined, thisArg?: TThis | undefined) => TResult;
export { setTimeoutWithContext as setTimeout };
declare function setTimeoutWithContext(callback: () => any, delay: number): NodeJS.Timer;
export declare function asyncFromGen<TArgs extends any[], TResult>(genFn: (...args: TArgs) => IterableIterator<TResult>): (...args: TArgs) => Promise<TResult>;
export declare function wrapYieldingFiberMethods<F extends Function>(Fiber: F): F;
