import { ZenObservable } from './types';
export { ZenObservable };
export declare type Observer<T> = ZenObservable.Observer<T>;
export declare type Subscriber<T> = ZenObservable.Subscriber<T>;
export declare type ObservableLike<T> = ZenObservable.ObservableLike<T>;
export declare const Observable: {
    new <T>(subscriber: Subscriber<T>): Observable<T>;
    from<R>(observable: Observable<R> | ZenObservable.ObservableLike<R> | ArrayLike<R>): Observable<R>;
    of<R>(...args: Array<R>): Observable<R>;
};
export interface Observable<T> {
    subscribe(observerOrNext: ((value: T) => void) | ZenObservable.Observer<T>, error?: (error: any) => void, complete?: () => void): ZenObservable.Subscription;
    forEach(fn: (value: T) => void): Promise<void>;
    map<R>(fn: (value: T) => R): Observable<R>;
    filter(fn: (value: T) => boolean): Observable<T>;
    reduce<R = T>(fn: (previousValue: R | T, currentValue: T) => R | T, initialValue?: R | T): Observable<R | T>;
    flatMap<R>(fn: (value: T) => ZenObservable.ObservableLike<R>): Observable<R>;
    from<R>(observable: Observable<R> | ZenObservable.ObservableLike<R> | ArrayLike<R>): Observable<R>;
    of<R>(...args: Array<R>): Observable<R>;
}
//# sourceMappingURL=zenObservable.d.ts.map