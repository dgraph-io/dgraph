import { NormalizedCache, NormalizedCacheObject, StoreObject } from './types';
export declare class MapCache implements NormalizedCache {
    private cache;
    constructor(data?: NormalizedCacheObject);
    get(dataId: string): StoreObject;
    set(dataId: string, value: StoreObject): void;
    delete(dataId: string): void;
    clear(): void;
    toObject(): NormalizedCacheObject;
    replace(newData: NormalizedCacheObject): void;
}
export declare function mapNormalizedCacheFactory(seed?: NormalizedCacheObject): NormalizedCache;
//# sourceMappingURL=mapCache.d.ts.map