import Observable from 'zen-observable-ts';
import { GraphQLRequest, Operation } from './types';
import { ApolloLink } from './link';
import { getOperationName } from 'apollo-utilities';
export { getOperationName };
export declare function validateOperation(operation: GraphQLRequest): GraphQLRequest;
export declare class LinkError extends Error {
    link: ApolloLink;
    constructor(message?: string, link?: ApolloLink);
}
export declare function isTerminating(link: ApolloLink): boolean;
export declare function toPromise<R>(observable: Observable<R>): Promise<R>;
export declare const makePromise: typeof toPromise;
export declare function fromPromise<T>(promise: Promise<T>): Observable<T>;
export declare function fromError<T>(errorValue: any): Observable<T>;
export declare function transformOperation(operation: GraphQLRequest): GraphQLRequest;
export declare function createOperation(starting: any, operation: GraphQLRequest): Operation;
export declare function getKey(operation: GraphQLRequest): string;
//# sourceMappingURL=linkUtils.d.ts.map