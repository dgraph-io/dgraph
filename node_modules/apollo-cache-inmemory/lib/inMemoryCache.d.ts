import './fixPolyfills';
import { DocumentNode } from 'graphql';
import { Cache, ApolloCache, Transaction } from 'apollo-cache';
import { ApolloReducerConfig, NormalizedCache, NormalizedCacheObject } from './types';
import { ObjectCache } from './objectCache';
export interface InMemoryCacheConfig extends ApolloReducerConfig {
    resultCaching?: boolean;
    freezeResults?: boolean;
}
export declare function defaultDataIdFromObject(result: any): string | null;
export declare class OptimisticCacheLayer extends ObjectCache {
    readonly optimisticId: string;
    readonly parent: NormalizedCache;
    readonly transaction: Transaction<NormalizedCacheObject>;
    constructor(optimisticId: string, parent: NormalizedCache, transaction: Transaction<NormalizedCacheObject>);
    toObject(): NormalizedCacheObject;
    get(dataId: string): import("./types").StoreObject;
}
export declare class InMemoryCache extends ApolloCache<NormalizedCacheObject> {
    private data;
    private optimisticData;
    protected config: InMemoryCacheConfig;
    private watches;
    private addTypename;
    private typenameDocumentCache;
    private storeReader;
    private storeWriter;
    private cacheKeyRoot;
    private silenceBroadcast;
    constructor(config?: InMemoryCacheConfig);
    restore(data: NormalizedCacheObject): this;
    extract(optimistic?: boolean): NormalizedCacheObject;
    read<T>(options: Cache.ReadOptions): T | null;
    write(write: Cache.WriteOptions): void;
    diff<T>(query: Cache.DiffOptions): Cache.DiffResult<T>;
    watch(watch: Cache.WatchOptions): () => void;
    evict(query: Cache.EvictOptions): Cache.EvictionResult;
    reset(): Promise<void>;
    removeOptimistic(idToRemove: string): void;
    performTransaction(transaction: Transaction<NormalizedCacheObject>, optimisticId?: string): void;
    recordOptimisticTransaction(transaction: Transaction<NormalizedCacheObject>, id: string): void;
    transformDocument(document: DocumentNode): DocumentNode;
    protected broadcastWatches(): void;
    private maybeBroadcastWatch;
}
//# sourceMappingURL=inMemoryCache.d.ts.map