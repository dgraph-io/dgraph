import { SelectionSetNode, FieldNode, DocumentNode } from 'graphql';
import { FragmentMatcher } from './readFromStore';
import { FragmentMap } from 'apollo-utilities';
import { IdGetter, NormalizedCache } from './types';
export declare class WriteError extends Error {
    type: string;
}
export declare function enhanceErrorWithDocument(error: Error, document: DocumentNode): WriteError;
export declare type WriteContext = {
    readonly store: NormalizedCache;
    readonly processedData?: {
        [x: string]: FieldNode[];
    };
    readonly variables?: any;
    readonly dataIdFromObject?: IdGetter;
    readonly fragmentMap?: FragmentMap;
    readonly fragmentMatcherFunction?: FragmentMatcher;
};
export declare class StoreWriter {
    writeQueryToStore({ query, result, store, variables, dataIdFromObject, fragmentMatcherFunction, }: {
        query: DocumentNode;
        result: Object;
        store?: NormalizedCache;
        variables?: Object;
        dataIdFromObject?: IdGetter;
        fragmentMatcherFunction?: FragmentMatcher;
    }): NormalizedCache;
    writeResultToStore({ dataId, result, document, store, variables, dataIdFromObject, fragmentMatcherFunction, }: {
        dataId: string;
        result: any;
        document: DocumentNode;
        store?: NormalizedCache;
        variables?: Object;
        dataIdFromObject?: IdGetter;
        fragmentMatcherFunction?: FragmentMatcher;
    }): NormalizedCache;
    writeSelectionSetToStore({ result, dataId, selectionSet, context, }: {
        dataId: string;
        result: any;
        selectionSet: SelectionSetNode;
        context: WriteContext;
    }): NormalizedCache;
    private writeFieldToStore;
    private processArrayValue;
}
//# sourceMappingURL=writeToStore.d.ts.map