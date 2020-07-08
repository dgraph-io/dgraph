import { IdValue } from 'apollo-utilities';
import { ReadStoreContext, FragmentMatcherInterface, IntrospectionResultData } from './types';
export declare class HeuristicFragmentMatcher implements FragmentMatcherInterface {
    constructor();
    ensureReady(): Promise<void>;
    canBypassInit(): boolean;
    match(idValue: IdValue, typeCondition: string, context: ReadStoreContext): boolean | 'heuristic';
}
export declare class IntrospectionFragmentMatcher implements FragmentMatcherInterface {
    private isReady;
    private possibleTypesMap;
    constructor(options?: {
        introspectionQueryResultData?: IntrospectionResultData;
    });
    match(idValue: IdValue, typeCondition: string, context: ReadStoreContext): boolean;
    private parseIntrospectionResult;
}
//# sourceMappingURL=fragmentMatcher.d.ts.map