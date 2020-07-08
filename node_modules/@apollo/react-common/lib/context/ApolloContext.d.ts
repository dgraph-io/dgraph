import React from 'react';
import ApolloClient from 'apollo-client';
export interface ApolloContextValue {
    client?: ApolloClient<object>;
    renderPromises?: Record<any, any>;
}
export declare function getApolloContext(): React.Context<ApolloContextValue>;
export declare function resetApolloContext(): void;
