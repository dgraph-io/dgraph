import { DataProxy } from './DataProxy';
export declare namespace Cache {
    type WatchCallback = (newData: any) => void;
    interface EvictionResult {
        success: Boolean;
    }
    interface ReadOptions<TVariables = any> extends DataProxy.Query<TVariables> {
        rootId?: string;
        previousResult?: any;
        optimistic: boolean;
    }
    interface WriteOptions<TResult = any, TVariables = any> extends DataProxy.Query<TVariables> {
        dataId: string;
        result: TResult;
    }
    interface DiffOptions extends ReadOptions {
        returnPartialData?: boolean;
    }
    interface WatchOptions extends ReadOptions {
        callback: WatchCallback;
    }
    interface EvictOptions<TVariables = any> extends DataProxy.Query<TVariables> {
        rootId?: string;
    }
    export import DiffResult = DataProxy.DiffResult;
    export import WriteQueryOptions = DataProxy.WriteQueryOptions;
    export import WriteFragmentOptions = DataProxy.WriteFragmentOptions;
    export import WriteDataOptions = DataProxy.WriteDataOptions;
    export import Fragment = DataProxy.Fragment;
}
//# sourceMappingURL=Cache.d.ts.map