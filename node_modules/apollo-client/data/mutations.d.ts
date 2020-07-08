import { DocumentNode } from 'graphql';
export declare class MutationStore {
    private store;
    getStore(): {
        [mutationId: string]: MutationStoreValue;
    };
    get(mutationId: string): MutationStoreValue;
    initMutation(mutationId: string, mutation: DocumentNode, variables: Object | undefined): void;
    markMutationError(mutationId: string, error: Error): void;
    markMutationResult(mutationId: string): void;
    reset(): void;
}
export interface MutationStoreValue {
    mutation: DocumentNode;
    variables: Object;
    loading: boolean;
    error: Error | null;
}
//# sourceMappingURL=mutations.d.ts.map