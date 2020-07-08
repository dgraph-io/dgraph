export declare namespace ZenObservable {
    interface SubscriptionObserver<T> {
        closed: boolean;
        next(value: T): void;
        error(errorValue: any): void;
        complete(): void;
    }
    interface Subscription {
        closed: boolean;
        unsubscribe(): void;
    }
    interface Observer<T> {
        start?(subscription: Subscription): any;
        next?(value: T): void;
        error?(errorValue: any): void;
        complete?(): void;
    }
    type Subscriber<T> = (observer: SubscriptionObserver<T>) => void | (() => void) | Subscription;
    interface ObservableLike<T> {
        subscribe?: Subscriber<T>;
    }
}
//# sourceMappingURL=types.d.ts.map