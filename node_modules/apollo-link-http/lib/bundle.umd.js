(function (global, factory) {
  typeof exports === 'object' && typeof module !== 'undefined' ? factory(exports, require('tslib'), require('apollo-link'), require('apollo-link-http-common')) :
  typeof define === 'function' && define.amd ? define(['exports', 'tslib', 'apollo-link', 'apollo-link-http-common'], factory) :
  (global = global || self, factory((global.apolloLink = global.apolloLink || {}, global.apolloLink.http = {}), global.tslib, global.apolloLink.core, global.apolloLink.httpCommon));
}(this, (function (exports, tslib_1, apolloLink, apolloLinkHttpCommon) { 'use strict';

  var createHttpLink = function (linkOptions) {
      if (linkOptions === void 0) { linkOptions = {}; }
      var _a = linkOptions.uri, uri = _a === void 0 ? '/graphql' : _a, fetcher = linkOptions.fetch, includeExtensions = linkOptions.includeExtensions, useGETForQueries = linkOptions.useGETForQueries, requestOptions = tslib_1.__rest(linkOptions, ["uri", "fetch", "includeExtensions", "useGETForQueries"]);
      apolloLinkHttpCommon.checkFetcher(fetcher);
      if (!fetcher) {
          fetcher = fetch;
      }
      var linkConfig = {
          http: { includeExtensions: includeExtensions },
          options: requestOptions.fetchOptions,
          credentials: requestOptions.credentials,
          headers: requestOptions.headers,
      };
      return new apolloLink.ApolloLink(function (operation) {
          var chosenURI = apolloLinkHttpCommon.selectURI(operation, uri);
          var context = operation.getContext();
          var clientAwarenessHeaders = {};
          if (context.clientAwareness) {
              var _a = context.clientAwareness, name_1 = _a.name, version = _a.version;
              if (name_1) {
                  clientAwarenessHeaders['apollographql-client-name'] = name_1;
              }
              if (version) {
                  clientAwarenessHeaders['apollographql-client-version'] = version;
              }
          }
          var contextHeaders = tslib_1.__assign({}, clientAwarenessHeaders, context.headers);
          var contextConfig = {
              http: context.http,
              options: context.fetchOptions,
              credentials: context.credentials,
              headers: contextHeaders,
          };
          var _b = apolloLinkHttpCommon.selectHttpOptionsAndBody(operation, apolloLinkHttpCommon.fallbackHttpConfig, linkConfig, contextConfig), options = _b.options, body = _b.body;
          var controller;
          if (!options.signal) {
              var _c = apolloLinkHttpCommon.createSignalIfSupported(), _controller = _c.controller, signal = _c.signal;
              controller = _controller;
              if (controller)
                  options.signal = signal;
          }
          var definitionIsMutation = function (d) {
              return d.kind === 'OperationDefinition' && d.operation === 'mutation';
          };
          if (useGETForQueries &&
              !operation.query.definitions.some(definitionIsMutation)) {
              options.method = 'GET';
          }
          if (options.method === 'GET') {
              var _d = rewriteURIForGET(chosenURI, body), newURI = _d.newURI, parseError = _d.parseError;
              if (parseError) {
                  return apolloLink.fromError(parseError);
              }
              chosenURI = newURI;
          }
          else {
              try {
                  options.body = apolloLinkHttpCommon.serializeFetchParameter(body, 'Payload');
              }
              catch (parseError) {
                  return apolloLink.fromError(parseError);
              }
          }
          return new apolloLink.Observable(function (observer) {
              fetcher(chosenURI, options)
                  .then(function (response) {
                  operation.setContext({ response: response });
                  return response;
              })
                  .then(apolloLinkHttpCommon.parseAndCheckHttpResponse(operation))
                  .then(function (result) {
                  observer.next(result);
                  observer.complete();
                  return result;
              })
                  .catch(function (err) {
                  if (err.name === 'AbortError')
                      return;
                  if (err.result && err.result.errors && err.result.data) {
                      observer.next(err.result);
                  }
                  observer.error(err);
              });
              return function () {
                  if (controller)
                      controller.abort();
              };
          });
      });
  };
  function rewriteURIForGET(chosenURI, body) {
      var queryParams = [];
      var addQueryParam = function (key, value) {
          queryParams.push(key + "=" + encodeURIComponent(value));
      };
      if ('query' in body) {
          addQueryParam('query', body.query);
      }
      if (body.operationName) {
          addQueryParam('operationName', body.operationName);
      }
      if (body.variables) {
          var serializedVariables = void 0;
          try {
              serializedVariables = apolloLinkHttpCommon.serializeFetchParameter(body.variables, 'Variables map');
          }
          catch (parseError) {
              return { parseError: parseError };
          }
          addQueryParam('variables', serializedVariables);
      }
      if (body.extensions) {
          var serializedExtensions = void 0;
          try {
              serializedExtensions = apolloLinkHttpCommon.serializeFetchParameter(body.extensions, 'Extensions map');
          }
          catch (parseError) {
              return { parseError: parseError };
          }
          addQueryParam('extensions', serializedExtensions);
      }
      var fragment = '', preFragment = chosenURI;
      var fragmentStart = chosenURI.indexOf('#');
      if (fragmentStart !== -1) {
          fragment = chosenURI.substr(fragmentStart);
          preFragment = chosenURI.substr(0, fragmentStart);
      }
      var queryParamsPrefix = preFragment.indexOf('?') === -1 ? '?' : '&';
      var newURI = preFragment + queryParamsPrefix + queryParams.join('&') + fragment;
      return { newURI: newURI };
  }
  var HttpLink = (function (_super) {
      tslib_1.__extends(HttpLink, _super);
      function HttpLink(opts) {
          return _super.call(this, createHttpLink(opts).request) || this;
      }
      return HttpLink;
  }(apolloLink.ApolloLink));

  exports.HttpLink = HttpLink;
  exports.createHttpLink = createHttpLink;

  Object.defineProperty(exports, '__esModule', { value: true });

})));
//# sourceMappingURL=bundle.umd.js.map
