import { DocumentNode } from 'graphql';
import { OperationVariables } from '@apollo/react-common';
import { SubscriptionHookOptions } from './types';
export declare function useSubscription<TData = any, TVariables = OperationVariables>(subscription: DocumentNode, options?: SubscriptionHookOptions<TData, TVariables>): {
    variables: TVariables | undefined;
    loading: boolean;
    data?: TData | undefined;
    error?: import("apollo-client").ApolloError | undefined;
};
