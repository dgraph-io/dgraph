/// <reference types="react" />
import { ObservableQuery } from 'apollo-client';
import { QueryOptions } from '../types';
import { QueryData } from '../data/QueryData';
export declare class RenderPromises {
    private queryPromises;
    private queryInfoTrie;
    registerSSRObservable<TData, TVariables>(observable: ObservableQuery<any, TVariables>, props: QueryOptions<TData, TVariables>): void;
    getSSRObservable<TData, TVariables>(props: QueryOptions<TData, TVariables>): ObservableQuery<any, any> | null;
    addQueryPromise<TData, TVariables>(queryInstance: QueryData<TData, TVariables>, finish: () => React.ReactNode): React.ReactNode;
    hasPromises(): boolean;
    consumeAndAwaitPromises(): Promise<any[]>;
    private lookupQueryInfo;
}
