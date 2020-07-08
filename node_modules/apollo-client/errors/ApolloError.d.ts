import { GraphQLError } from 'graphql';
export declare function isApolloError(err: Error): err is ApolloError;
export declare class ApolloError extends Error {
    message: string;
    graphQLErrors: ReadonlyArray<GraphQLError>;
    networkError: Error | null;
    extraInfo: any;
    constructor({ graphQLErrors, networkError, errorMessage, extraInfo, }: {
        graphQLErrors?: ReadonlyArray<GraphQLError>;
        networkError?: Error | null;
        errorMessage?: string;
        extraInfo?: any;
    });
}
//# sourceMappingURL=ApolloError.d.ts.map