import { Observable } from './Observable';
export declare function multiplex<T>(inner: Observable<T>): Observable<T>;
export declare function asyncMap<V, R>(observable: Observable<V>, mapFn: (value: V) => R | Promise<R>): Observable<R>;
//# sourceMappingURL=observables.d.ts.map