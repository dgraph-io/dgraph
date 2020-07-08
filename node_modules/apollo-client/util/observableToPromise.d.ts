import { ObservableQuery } from '../core/ObservableQuery';
import { ApolloQueryResult } from '../core/types';
import { Subscription } from '../util/Observable';
export declare type Options = {
    observable: ObservableQuery<any>;
    shouldResolve?: boolean;
    wait?: number;
    errorCallbacks?: ((error: Error) => any)[];
};
export declare type ResultCallback = ((result: ApolloQueryResult<any>) => any);
export declare function observableToPromiseAndSubscription({ observable, shouldResolve, wait, errorCallbacks }: Options, ...cbs: ResultCallback[]): {
    promise: Promise<any[]>;
    subscription: Subscription;
};
export default function (options: Options, ...cbs: ResultCallback[]): Promise<any[]>;
//# sourceMappingURL=observableToPromise.d.ts.map