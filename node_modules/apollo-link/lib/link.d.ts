import Observable from 'zen-observable-ts';
import { GraphQLRequest, NextLink, Operation, RequestHandler, FetchResult } from './types';
export declare function empty(): ApolloLink;
export declare function from(links: ApolloLink[]): ApolloLink;
export declare function split(test: (op: Operation) => boolean, left: ApolloLink | RequestHandler, right?: ApolloLink | RequestHandler): ApolloLink;
export declare const concat: (first: RequestHandler | ApolloLink, second: RequestHandler | ApolloLink) => ApolloLink;
export declare class ApolloLink {
    static empty: typeof empty;
    static from: typeof from;
    static split: typeof split;
    static execute: typeof execute;
    constructor(request?: RequestHandler);
    split(test: (op: Operation) => boolean, left: ApolloLink | RequestHandler, right?: ApolloLink | RequestHandler): ApolloLink;
    concat(next: ApolloLink | RequestHandler): ApolloLink;
    request(operation: Operation, forward?: NextLink): Observable<FetchResult> | null;
}
export declare function execute(link: ApolloLink, operation: GraphQLRequest): Observable<FetchResult>;
//# sourceMappingURL=link.d.ts.map