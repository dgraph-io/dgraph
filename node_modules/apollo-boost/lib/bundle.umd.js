(function (global, factory) {
  if (typeof define === "function" && define.amd) {
    define(["exports", "tslib", "apollo-client", "apollo-link", "apollo-cache-inmemory", "apollo-link-http", "apollo-link-error", "graphql-tag", "ts-invariant"], factory);
  } else if (typeof exports !== "undefined") {
    factory(exports, require("tslib"), require("apollo-client"), require("apollo-link"), require("apollo-cache-inmemory"), require("apollo-link-http"), require("apollo-link-error"), require("graphql-tag"), require("ts-invariant"));
  } else {
    var mod = {
      exports: {}
    };
    factory(mod.exports, global.tslib, global.apolloClient, global.apolloLink, global.apolloCacheInmemory, global.apolloLinkHttp, global.apolloLinkError, global.graphqlTag, global.tsInvariant);
    global.unknown = mod.exports;
  }
})(typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : this, function (_exports, _tslib, _apolloClient, _apolloLink, _apolloCacheInmemory, _apolloLinkHttp, _apolloLinkError, _graphqlTag, _tsInvariant) {

  _exports.__esModule = true;
  var _exportNames = {
    gql: true,
    HttpLink: true
  };
  _exports.default = _exports.gql = void 0;
  _apolloClient = _interopRequireWildcard(_apolloClient);
  Object.keys(_apolloClient).forEach(function (key) {
    if (key === "default" || key === "__esModule") return;
    if (Object.prototype.hasOwnProperty.call(_exportNames, key)) return;
    _exports[key] = _apolloClient[key];
  });
  Object.keys(_apolloLink).forEach(function (key) {
    if (key === "default" || key === "__esModule") return;
    if (Object.prototype.hasOwnProperty.call(_exportNames, key)) return;
    _exports[key] = _apolloLink[key];
  });
  Object.keys(_apolloCacheInmemory).forEach(function (key) {
    if (key === "default" || key === "__esModule") return;
    if (Object.prototype.hasOwnProperty.call(_exportNames, key)) return;
    _exports[key] = _apolloCacheInmemory[key];
  });
  _exports.HttpLink = _apolloLinkHttp.HttpLink;
  _graphqlTag = _interopRequireDefault(_graphqlTag);
  _exports.gql = _graphqlTag.default;

  function _interopRequireDefault(obj) { return obj && obj.__esModule ? obj : { default: obj }; }

  function _getRequireWildcardCache() { if (typeof WeakMap !== "function") return null; var cache = new WeakMap(); _getRequireWildcardCache = function () { return cache; }; return cache; }

  function _interopRequireWildcard(obj) { if (obj && obj.__esModule) { return obj; } if (obj === null || typeof obj !== "object" && typeof obj !== "function") { return { default: obj }; } var cache = _getRequireWildcardCache(); if (cache && cache.has(obj)) { return cache.get(obj); } var newObj = {}; var hasPropertyDescriptor = Object.defineProperty && Object.getOwnPropertyDescriptor; for (var key in obj) { if (Object.prototype.hasOwnProperty.call(obj, key)) { var desc = hasPropertyDescriptor ? Object.getOwnPropertyDescriptor(obj, key) : null; if (desc && (desc.get || desc.set)) { Object.defineProperty(newObj, key, desc); } else { newObj[key] = obj[key]; } } } newObj.default = obj; if (cache) { cache.set(obj, newObj); } return newObj; }

  var PRESET_CONFIG_KEYS = ['request', 'uri', 'credentials', 'headers', 'fetch', 'fetchOptions', 'clientState', 'onError', 'cacheRedirects', 'cache', 'name', 'version', 'resolvers', 'typeDefs', 'fragmentMatcher'];

  var DefaultClient = function (_super) {
    (0, _tslib.__extends)(DefaultClient, _super);

    function DefaultClient(config) {
      if (config === void 0) {
        config = {};
      }

      var _this = this;

      if (config) {
        var diff = Object.keys(config).filter(function (key) {
          return PRESET_CONFIG_KEYS.indexOf(key) === -1;
        });

        if (diff.length > 0) {
          process.env.NODE_ENV === "production" || _tsInvariant.invariant.warn('ApolloBoost was initialized with unsupported options: ' + ("" + diff.join(' ')));
        }
      }

      var request = config.request,
          uri = config.uri,
          credentials = config.credentials,
          headers = config.headers,
          fetch = config.fetch,
          fetchOptions = config.fetchOptions,
          clientState = config.clientState,
          cacheRedirects = config.cacheRedirects,
          errorCallback = config.onError,
          name = config.name,
          version = config.version,
          resolvers = config.resolvers,
          typeDefs = config.typeDefs,
          fragmentMatcher = config.fragmentMatcher;
      var cache = config.cache;
      process.env.NODE_ENV === "production" ? (0, _tsInvariant.invariant)(!cache || !cacheRedirects, 1) : (0, _tsInvariant.invariant)(!cache || !cacheRedirects, 'Incompatible cache configuration. When not providing `cache`, ' + 'configure the provided instance with `cacheRedirects` instead.');

      if (!cache) {
        cache = cacheRedirects ? new _apolloCacheInmemory.InMemoryCache({
          cacheRedirects: cacheRedirects
        }) : new _apolloCacheInmemory.InMemoryCache();
      }

      var errorLink = errorCallback ? (0, _apolloLinkError.onError)(errorCallback) : (0, _apolloLinkError.onError)(function (_a) {
        var graphQLErrors = _a.graphQLErrors,
            networkError = _a.networkError;

        if (graphQLErrors) {
          graphQLErrors.forEach(function (_a) {
            var message = _a.message,
                locations = _a.locations,
                path = _a.path;
            return process.env.NODE_ENV === "production" || _tsInvariant.invariant.warn("[GraphQL error]: Message: " + message + ", Location: " + (locations + ", Path: " + path));
          });
        }

        if (networkError) {
          process.env.NODE_ENV === "production" || _tsInvariant.invariant.warn("[Network error]: " + networkError);
        }
      });
      var requestHandler = request ? new _apolloLink.ApolloLink(function (operation, forward) {
        return new _apolloLink.Observable(function (observer) {
          var handle;
          Promise.resolve(operation).then(function (oper) {
            return request(oper);
          }).then(function () {
            handle = forward(operation).subscribe({
              next: observer.next.bind(observer),
              error: observer.error.bind(observer),
              complete: observer.complete.bind(observer)
            });
          }).catch(observer.error.bind(observer));
          return function () {
            if (handle) {
              handle.unsubscribe();
            }
          };
        });
      }) : false;
      var httpLink = new _apolloLinkHttp.HttpLink({
        uri: uri || '/graphql',
        fetch: fetch,
        fetchOptions: fetchOptions || {},
        credentials: credentials || 'same-origin',
        headers: headers || {}
      });

      var link = _apolloLink.ApolloLink.from([errorLink, requestHandler, httpLink].filter(function (x) {
        return !!x;
      }));

      var activeResolvers = resolvers;
      var activeTypeDefs = typeDefs;
      var activeFragmentMatcher = fragmentMatcher;

      if (clientState) {
        if (clientState.defaults) {
          cache.writeData({
            data: clientState.defaults
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
        fragmentMatcher: activeFragmentMatcher
      }) || this;
      return _this;
    }

    return DefaultClient;
  }(_apolloClient.default);

  var _default = DefaultClient; 

  _exports.default = _default;
});
