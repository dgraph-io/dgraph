import { DataProxy } from './DataProxy';

export namespace Cache {
  export type WatchCallback = (newData: any) => void;
  export interface EvictionResult {
    success: Boolean;
  }

  export interface ReadOptions<TVariables = any>
    extends DataProxy.Query<TVariables> {
    rootId?: string;
    previousResult?: any;
    optimistic: boolean;
  }

  export interface WriteOptions<TResult = any, TVariables = any>
    extends DataProxy.Query<TVariables> {
    dataId: string;
    result: TResult;
  }

  export interface DiffOptions extends ReadOptions {
    returnPartialData?: boolean;
  }

  export interface WatchOptions extends ReadOptions {
    callback: WatchCallback;
  }

  export interface EvictOptions<TVariables = any>
    extends DataProxy.Query<TVariables> {
    rootId?: string;
  }

  export import DiffResult = DataProxy.DiffResult;
  export import WriteQueryOptions = DataProxy.WriteQueryOptions;
  export import WriteFragmentOptions = DataProxy.WriteFragmentOptions;
  export import WriteDataOptions = DataProxy.WriteDataOptions;
  export import Fragment = DataProxy.Fragment;
}
