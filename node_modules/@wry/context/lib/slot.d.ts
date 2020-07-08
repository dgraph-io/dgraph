declare const makeSlotClass: () => {
    new <TValue>(): {
        readonly id: string;
        hasValue(): boolean;
        getValue(): TValue | undefined;
        withValue<TResult, TArgs extends any[], TThis = any>(value: TValue, callback: (this: TThis, ...args: TArgs) => TResult, args?: TArgs | undefined, thisArg?: TThis | undefined): TResult;
    };
    bind<TArgs extends any[], TResult>(callback: (...args: TArgs) => TResult): (...args: TArgs) => TResult;
    noContext<TResult, TArgs extends any[], TThis = any>(callback: (this: TThis, ...args: TArgs) => TResult, args?: TArgs | undefined, thisArg?: TThis | undefined): TResult;
};
export declare const Slot: ReturnType<typeof makeSlotClass>;
export {};
