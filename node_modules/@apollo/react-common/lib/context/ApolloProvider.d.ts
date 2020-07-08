import React from 'react';
import ApolloClient from 'apollo-client';
export interface ApolloProviderProps<TCache> {
    client: ApolloClient<TCache>;
    children: React.ReactNode | React.ReactNode[] | null;
}
export declare const ApolloProvider: React.FC<ApolloProviderProps<any>>;
