(function (global, factory) {
  typeof exports === 'object' && typeof module !== 'undefined' ? factory(exports, require('tslib'), require('apollo-link')) :
  typeof define === 'function' && define.amd ? define(['exports', 'tslib', 'apollo-link'], factory) :
  (global = global || self, factory((global.apolloLink = global.apolloLink || {}, global.apolloLink.error = {}), global.tslib, global.apolloLink.core));
}(this, (function (exports, tslib_1, apolloLink) { 'use strict';

  function onError(errorHandler) {
      return new apolloLink.ApolloLink(function (operation, forward) {
          return new apolloLink.Observable(function (observer) {
              var sub;
              var retriedSub;
              var retriedResult;
              try {
                  sub = forward(operation).subscribe({
                      next: function (result) {
                          if (result.errors) {
                              retriedResult = errorHandler({
                                  graphQLErrors: result.errors,
                                  response: result,
                                  operation: operation,
                                  forward: forward,
                              });
                              if (retriedResult) {
                                  retriedSub = retriedResult.subscribe({
                                      next: observer.next.bind(observer),
                                      error: observer.error.bind(observer),
                                      complete: observer.complete.bind(observer),
                                  });
                                  return;
                              }
                          }
                          observer.next(result);
                      },
                      error: function (networkError) {
                          retriedResult = errorHandler({
                              operation: operation,
                              networkError: networkError,
                              graphQLErrors: networkError &&
                                  networkError.result &&
                                  networkError.result.errors,
                              forward: forward,
                          });
                          if (retriedResult) {
                              retriedSub = retriedResult.subscribe({
                                  next: observer.next.bind(observer),
                                  error: observer.error.bind(observer),
                                  complete: observer.complete.bind(observer),
                              });
                              return;
                          }
                          observer.error(networkError);
                      },
                      complete: function () {
                          if (!retriedResult) {
                              observer.complete.bind(observer)();
                          }
                      },
                  });
              }
              catch (e) {
                  errorHandler({ networkError: e, operation: operation, forward: forward });
                  observer.error(e);
              }
              return function () {
                  if (sub)
                      sub.unsubscribe();
                  if (retriedSub)
                      sub.unsubscribe();
              };
          });
      });
  }
  var ErrorLink = (function (_super) {
      tslib_1.__extends(ErrorLink, _super);
      function ErrorLink(errorHandler) {
          var _this = _super.call(this) || this;
          _this.link = onError(errorHandler);
          return _this;
      }
      ErrorLink.prototype.request = function (operation, forward) {
          return this.link.request(operation, forward);
      };
      return ErrorLink;
  }(apolloLink.ApolloLink));

  exports.ErrorLink = ErrorLink;
  exports.onError = onError;

  Object.defineProperty(exports, '__esModule', { value: true });

})));
//# sourceMappingURL=bundle.umd.js.map
