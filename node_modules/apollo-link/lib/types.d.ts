import Observable from 'zen-observable-ts';
import { DocumentNode } from 'graphql/language/ast';
import { ExecutionResult as GraphQLExecutionResult } from 'graphql';
export { DocumentNode };
export interface ExecutionResult<TData = {
    [key: string]: any;
}> extends GraphQLExecutionResult {
    data?: TData | null;
}
export interface GraphQLRequest {
    query: DocumentNode;
    variables?: Record<string, any>;
    operationName?: string;
    context?: Record<string, any>;
    extensions?: Record<string, any>;
}
export interface Operation {
    query: DocumentNode;
    variables: Record<string, any>;
    operationName: string;
    extensions: Record<string, any>;
    setContext: (context: Record<string, any>) => Record<string, any>;
    getContext: () => Record<string, any>;
    toKey: () => string;
}
export declare type FetchResult<TData = {
    [key: string]: any;
}, C = Record<string, any>, E = Record<string, any>> = ExecutionResult<TData> & {
    extensions?: E;
    context?: C;
};
export declare type NextLink = (operation: Operation) => Observable<FetchResult>;
export declare type RequestHandler = (operation: Operation, forward: NextLink) => Observable<FetchResult> | null;
//# sourceMappingURL=types.d.ts.map