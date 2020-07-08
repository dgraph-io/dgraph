import { Operation, RequestHandler, NextLink, FetchResult } from '../types';
import Observable from 'zen-observable-ts';
import { ApolloLink } from '../link';
export default class MockLink extends ApolloLink {
    constructor(handleRequest?: RequestHandler);
    request(operation: Operation, forward?: NextLink): Observable<FetchResult> | null;
}
//# sourceMappingURL=mockLink.d.ts.map