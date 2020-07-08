import { Operation, NextLink, FetchResult } from '../types';
import Observable from 'zen-observable-ts';
import { ApolloLink } from '../link';
export default class SetContextLink extends ApolloLink {
    private setContext;
    constructor(setContext?: (context: Record<string, any>) => Record<string, any>);
    request(operation: Operation, forward: NextLink): Observable<FetchResult>;
}
//# sourceMappingURL=setContext.d.ts.map