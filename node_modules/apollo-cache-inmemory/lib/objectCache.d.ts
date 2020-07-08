import { NormalizedCache, NormalizedCacheObject, StoreObject } from './types';
export declare class ObjectCache implements NormalizedCache {
    protected data: NormalizedCacheObject;
    constructor(data?: NormalizedCacheObject);
    toObject(): NormalizedCacheObject;
    get(dataId: string): StoreObject;
    set(dataId: string, value: StoreObject): void;
    delete(dataId: string): void;
    clear(): void;
    replace(newData: NormalizedCacheObject): void;
}
export declare function defaultNormalizedCacheFactory(seed?: NormalizedCacheObject): NormalizedCache;
//# sourceMappingURL=objectCache.d.ts.map