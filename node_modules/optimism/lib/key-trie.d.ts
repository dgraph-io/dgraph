export declare class KeyTrie<K> {
    private readonly weakness;
    private weak?;
    private strong?;
    private data?;
    constructor(weakness: boolean);
    lookup<T extends any[]>(...array: T): K;
    lookupArray<T extends any[]>(array: T): K;
    private getChildTrie;
}
