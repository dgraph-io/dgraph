import React from 'react';
import ApolloClient from 'apollo-client';
export interface ApolloConsumerProps {
    children: (client: ApolloClient<object>) => React.ReactChild | null;
}
export declare const ApolloConsumer: React.FC<ApolloConsumerProps>;
