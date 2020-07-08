import { ApolloLink, RequestHandler } from 'apollo-link';
import { HttpOptions, UriFunction as _UriFunction } from 'apollo-link-http-common';
export declare namespace HttpLink {
    interface UriFunction extends _UriFunction {
    }
    interface Options extends HttpOptions {
        useGETForQueries?: boolean;
    }
}
export import FetchOptions = HttpLink.Options;
export import UriFunction = HttpLink.UriFunction;
export declare const createHttpLink: (linkOptions?: FetchOptions) => ApolloLink;
export declare class HttpLink extends ApolloLink {
    requester: RequestHandler;
    constructor(opts?: HttpLink.Options);
}
//# sourceMappingURL=httpLink.d.ts.map