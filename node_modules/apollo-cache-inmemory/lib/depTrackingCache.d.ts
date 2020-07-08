import { NormalizedCache, NormalizedCacheObject, StoreObject } from './types';
export declare class DepTrackingCache implements NormalizedCache {
    private data;
    private depend;
    constructor(data?: NormalizedCacheObject);
    toObject(): NormalizedCacheObject;
    get(dataId: string): StoreObject;
    set(dataId: string, value?: StoreObject): void;
    delete(dataId: string): void;
    clear(): void;
    replace(newData: NormalizedCacheObject | null): void;
}
export declare function defaultNormalizedCacheFactory(seed?: NormalizedCacheObject): NormalizedCache;
//# sourceMappingURL=depTrackingCache.d.ts.map