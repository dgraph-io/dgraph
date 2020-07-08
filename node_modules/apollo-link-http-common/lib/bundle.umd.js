(function (global, factory) {
  typeof exports === 'object' && typeof module !== 'undefined' ? factory(exports, require('tslib'), require('graphql/language/printer'), require('ts-invariant')) :
  typeof define === 'function' && define.amd ? define(['exports', 'tslib', 'graphql/language/printer', 'ts-invariant'], factory) :
  (global = global || self, factory((global.apolloLink = global.apolloLink || {}, global.apolloLink.httpCommon = {}), global.tslib, global.graphql.printer, global.invariant));
}(this, (function (exports, tslib_1, printer, tsInvariant) { 'use strict';

  var defaultHttpOptions = {
      includeQuery: true,
      includeExtensions: false,
  };
  var defaultHeaders = {
      accept: '*/*',
      'content-type': 'application/json',
  };
  var defaultOptions = {
      method: 'POST',
  };
  var fallbackHttpConfig = {
      http: defaultHttpOptions,
      headers: defaultHeaders,
      options: defaultOptions,
  };
  var throwServerError = function (response, result, message) {
      var error = new Error(message);
      error.name = 'ServerError';
      error.response = response;
      error.statusCode = response.status;
      error.result = result;
      throw error;
  };
  var parseAndCheckHttpResponse = function (operations) { return function (response) {
      return (response
          .text()
          .then(function (bodyText) {
          try {
              return JSON.parse(bodyText);
          }
          catch (err) {
              var parseError = err;
              parseError.name = 'ServerParseError';
              parseError.response = response;
              parseError.statusCode = response.status;
              parseError.bodyText = bodyText;
              return Promise.reject(parseError);
          }
      })
          .then(function (result) {
          if (response.status >= 300) {
              throwServerError(response, result, "Response not successful: Received status code " + response.status);
          }
          if (!Array.isArray(result) &&
              !result.hasOwnProperty('data') &&
              !result.hasOwnProperty('errors')) {
              throwServerError(response, result, "Server response was missing for query '" + (Array.isArray(operations)
                  ? operations.map(function (op) { return op.operationName; })
                  : operations.operationName) + "'.");
          }
          return result;
      }));
  }; };
  var checkFetcher = function (fetcher) {
      if (!fetcher && typeof fetch === 'undefined') {
          var library = 'unfetch';
          if (typeof window === 'undefined')
              library = 'node-fetch';
          throw process.env.NODE_ENV === "production" ? new tsInvariant.InvariantError(1) : new tsInvariant.InvariantError("\nfetch is not found globally and no fetcher passed, to fix pass a fetch for\nyour environment like https://www.npmjs.com/package/" + library + ".\n\nFor example:\nimport fetch from '" + library + "';\nimport { createHttpLink } from 'apollo-link-http';\n\nconst link = createHttpLink({ uri: '/graphql', fetch: fetch });");
      }
  };
  var createSignalIfSupported = function () {
      if (typeof AbortController === 'undefined')
          return { controller: false, signal: false };
      var controller = new AbortController();
      var signal = controller.signal;
      return { controller: controller, signal: signal };
  };
  var selectHttpOptionsAndBody = function (operation, fallbackConfig) {
      var configs = [];
      for (var _i = 2; _i < arguments.length; _i++) {
          configs[_i - 2] = arguments[_i];
      }
      var options = tslib_1.__assign({}, fallbackConfig.options, { headers: fallbackConfig.headers, credentials: fallbackConfig.credentials });
      var http = fallbackConfig.http;
      configs.forEach(function (config) {
          options = tslib_1.__assign({}, options, config.options, { headers: tslib_1.__assign({}, options.headers, config.headers) });
          if (config.credentials)
              options.credentials = config.credentials;
          http = tslib_1.__assign({}, http, config.http);
      });
      var operationName = operation.operationName, extensions = operation.extensions, variables = operation.variables, query = operation.query;
      var body = { operationName: operationName, variables: variables };
      if (http.includeExtensions)
          body.extensions = extensions;
      if (http.includeQuery)
          body.query = printer.print(query);
      return {
          options: options,
          body: body,
      };
  };
  var serializeFetchParameter = function (p, label) {
      var serialized;
      try {
          serialized = JSON.stringify(p);
      }
      catch (e) {
          var parseError = process.env.NODE_ENV === "production" ? new tsInvariant.InvariantError(2) : new tsInvariant.InvariantError("Network request failed. " + label + " is not serializable: " + e.message);
          parseError.parseError = e;
          throw parseError;
      }
      return serialized;
  };
  var selectURI = function (operation, fallbackURI) {
      var context = operation.getContext();
      var contextURI = context.uri;
      if (contextURI) {
          return contextURI;
      }
      else if (typeof fallbackURI === 'function') {
          return fallbackURI(operation);
      }
      else {
          return fallbackURI || '/graphql';
      }
  };

  exports.checkFetcher = checkFetcher;
  exports.createSignalIfSupported = createSignalIfSupported;
  exports.fallbackHttpConfig = fallbackHttpConfig;
  exports.parseAndCheckHttpResponse = parseAndCheckHttpResponse;
  exports.selectHttpOptionsAndBody = selectHttpOptionsAndBody;
  exports.selectURI = selectURI;
  exports.serializeFetchParameter = serializeFetchParameter;
  exports.throwServerError = throwServerError;

  Object.defineProperty(exports, '__esModule', { value: true });

})));
//# sourceMappingURL=bundle.umd.js.map
