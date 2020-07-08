import { ApolloClient } from 'apollo-client';
import { ApolloContextValue, DocumentType } from '@apollo/react-common';
import { DocumentNode } from 'graphql';
import { CommonOptions } from '../types';
export declare abstract class OperationData<TOptions = any> {
    isMounted: boolean;
    previousOptions: CommonOptions<TOptions>;
    context: ApolloContextValue;
    client: ApolloClient<object> | undefined;
    private options;
    constructor(options?: CommonOptions<TOptions>, context?: ApolloContextValue);
    getOptions(): CommonOptions<TOptions>;
    setOptions(newOptions: CommonOptions<TOptions>, storePrevious?: boolean): void;
    abstract execute(...args: any): any;
    abstract afterExecute(...args: any): void | (() => void);
    abstract cleanup(): void;
    protected unmount(): void;
    protected refreshClient(): {
        client: ApolloClient<object>;
        isNew: boolean;
    };
    protected verifyDocumentType(document: DocumentNode, type: DocumentType): void;
}
