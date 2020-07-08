import { __extends } from 'tslib';
import ApolloClient__default from 'apollo-client';
export * from 'apollo-client';
import { ApolloLink, Observable } from 'apollo-link';
export * from 'apollo-link';
import { InMemoryCache } from 'apollo-cache-inmemory';
export * from 'apollo-cache-inmemory';
import { HttpLink } from 'apollo-link-http';
export { HttpLink } from 'apollo-link-http';
import { onError } from 'apollo-link-error';
export { default as gql } from 'graphql-tag';
import { invariant } from 'ts-invariant';

var PRESET_CONFIG_KEYS = [
    'request',
    'uri',
    'credentials',
    'headers',
    'fetch',
    'fetchOptions',
    'clientState',
    'onError',
    'cacheRedirects',
    'cache',
    'name',
    'version',
    'resolvers',
    'typeDefs',
    'fragmentMatcher',
];
var DefaultClient = (function (_super) {
    __extends(DefaultClient, _super);
    function DefaultClient(config) {
        if (config === void 0) { config = {}; }
        var _this = this;
        if (config) {
            var diff = Object.keys(config).filter(function (key) { return PRESET_CONFIG_KEYS.indexOf(key) === -1; });
            if (diff.length > 0) {
                process.env.NODE_ENV === "production" || invariant.warn('ApolloBoost was initialized with unsupported options: ' +
                    ("" + diff.join(' ')));
            }
        }
        var request = config.request, uri = config.uri, credentials = config.credentials, headers = config.headers, fetch = config.fetch, fetchOptions = config.fetchOptions, clientState = config.clientState, cacheRedirects = config.cacheRedirects, errorCallback = config.onError, name = config.name, version = config.version, resolvers = config.resolvers, typeDefs = config.typeDefs, fragmentMatcher = config.fragmentMatcher;
        var cache = config.cache;
        process.env.NODE_ENV === "production" ? invariant(!cache || !cacheRedirects, 1) : invariant(!cache || !cacheRedirects, 'Incompatible cache configuration. When not providing `cache`, ' +
            'configure the provided instance with `cacheRedirects` instead.');
        if (!cache) {
            cache = cacheRedirects
                ? new InMemoryCache({ cacheRedirects: cacheRedirects })
                : new InMemoryCache();
        }
        var errorLink = errorCallback
            ? onError(errorCallback)
            : onError(function (_a) {
                var graphQLErrors = _a.graphQLErrors, networkError = _a.networkError;
                if (graphQLErrors) {
                    graphQLErrors.forEach(function (_a) {
                        var message = _a.message, locations = _a.locations, path = _a.path;
                        return process.env.NODE_ENV === "production" || invariant.warn("[GraphQL error]: Message: " + message + ", Location: " +
                            (locations + ", Path: " + path));
                    });
                }
                if (networkError) {
                    process.env.NODE_ENV === "production" || invariant.warn("[Network error]: " + networkError);
                }
            });
        var requestHandler = request
            ? new ApolloLink(function (operation, forward) {
                return new Observable(function (observer) {
                    var handle;
                    Promise.resolve(operation)
                        .then(function (oper) { return request(oper); })
                        .then(function () {
                        handle = forward(operation).subscribe({
                            next: observer.next.bind(observer),
                            error: observer.error.bind(observer),
                            complete: observer.complete.bind(observer),
                        });
                    })
                        .catch(observer.error.bind(observer));
                    return function () {
                        if (handle) {
                            handle.unsubscribe();
                        }
                    };
                });
            })
            : false;
        var httpLink = new HttpLink({
            uri: uri || '/graphql',
            fetch: fetch,
            fetchOptions: fetchOptions || {},
            credentials: credentials || 'same-origin',
            headers: headers || {},
        });
        var link = ApolloLink.from([errorLink, requestHandler, httpLink].filter(function (x) { return !!x; }));
        var activeResolvers = resolvers;
        var activeTypeDefs = typeDefs;
        var activeFragmentMatcher = fragmentMatcher;
        if (clientState) {
            if (clientState.defaults) {
                cache.writeData({
                    data: clientState.defaults,
                });
            }
            activeResolvers = clientState.resolvers;
            activeTypeDefs = clientState.typeDefs;
            activeFragmentMatcher = clientState.fragmentMatcher;
        }
        _this = _super.call(this, {
            cache: cache,
            link: link,
            name: name,
            version: version,
            resolvers: activeResolvers,
            typeDefs: activeTypeDefs,
            fragmentMatcher: activeFragmentMatcher,
        }) || this;
        return _this;
    }
    return DefaultClient;
}(ApolloClient__default));

export default DefaultClient;
//# sourceMappingURL=bundle.esm.js.map
