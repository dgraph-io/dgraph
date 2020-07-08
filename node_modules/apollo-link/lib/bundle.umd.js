(function (global, factory) {
  typeof exports === 'object' && typeof module !== 'undefined' ? factory(exports, require('zen-observable-ts'), require('ts-invariant'), require('tslib'), require('apollo-utilities')) :
  typeof define === 'function' && define.amd ? define(['exports', 'zen-observable-ts', 'ts-invariant', 'tslib', 'apollo-utilities'], factory) :
  (global = global || self, factory((global.apolloLink = global.apolloLink || {}, global.apolloLink.core = {}), global.apolloLink.zenObservable, global.invariant, global.tslib, global.apolloUtilities));
}(this, (function (exports, Observable, tsInvariant, tslib_1, apolloUtilities) { 'use strict';

  Observable = Observable && Observable.hasOwnProperty('default') ? Observable['default'] : Observable;

  function validateOperation(operation) {
      var OPERATION_FIELDS = [
          'query',
          'operationName',
          'variables',
          'extensions',
          'context',
      ];
      for (var _i = 0, _a = Object.keys(operation); _i < _a.length; _i++) {
          var key = _a[_i];
          if (OPERATION_FIELDS.indexOf(key) < 0) {
              throw process.env.NODE_ENV === "production" ? new tsInvariant.InvariantError(2) : new tsInvariant.InvariantError("illegal argument: " + key);
          }
      }
      return operation;
  }
  var LinkError = (function (_super) {
      tslib_1.__extends(LinkError, _super);
      function LinkError(message, link) {
          var _this = _super.call(this, message) || this;
          _this.link = link;
          return _this;
      }
      return LinkError;
  }(Error));
  function isTerminating(link) {
      return link.request.length <= 1;
  }
  function toPromise(observable) {
      var completed = false;
      return new Promise(function (resolve, reject) {
          observable.subscribe({
              next: function (data) {
                  if (completed) {
                      process.env.NODE_ENV === "production" || tsInvariant.invariant.warn("Promise Wrapper does not support multiple results from Observable");
                  }
                  else {
                      completed = true;
                      resolve(data);
                  }
              },
              error: reject,
          });
      });
  }
  var makePromise = toPromise;
  function fromPromise(promise) {
      return new Observable(function (observer) {
          promise
              .then(function (value) {
              observer.next(value);
              observer.complete();
          })
              .catch(observer.error.bind(observer));
      });
  }
  function fromError(errorValue) {
      return new Observable(function (observer) {
          observer.error(errorValue);
      });
  }
  function transformOperation(operation) {
      var transformedOperation = {
          variables: operation.variables || {},
          extensions: operation.extensions || {},
          operationName: operation.operationName,
          query: operation.query,
      };
      if (!transformedOperation.operationName) {
          transformedOperation.operationName =
              typeof transformedOperation.query !== 'string'
                  ? apolloUtilities.getOperationName(transformedOperation.query)
                  : '';
      }
      return transformedOperation;
  }
  function createOperation(starting, operation) {
      var context = tslib_1.__assign({}, starting);
      var setContext = function (next) {
          if (typeof next === 'function') {
              context = tslib_1.__assign({}, context, next(context));
          }
          else {
              context = tslib_1.__assign({}, context, next);
          }
      };
      var getContext = function () { return (tslib_1.__assign({}, context)); };
      Object.defineProperty(operation, 'setContext', {
          enumerable: false,
          value: setContext,
      });
      Object.defineProperty(operation, 'getContext', {
          enumerable: false,
          value: getContext,
      });
      Object.defineProperty(operation, 'toKey', {
          enumerable: false,
          value: function () { return getKey(operation); },
      });
      return operation;
  }
  function getKey(operation) {
      var query = operation.query, variables = operation.variables, operationName = operation.operationName;
      return JSON.stringify([operationName, query, variables]);
  }

  function passthrough(op, forward) {
      return forward ? forward(op) : Observable.of();
  }
  function toLink(handler) {
      return typeof handler === 'function' ? new ApolloLink(handler) : handler;
  }
  function empty() {
      return new ApolloLink(function () { return Observable.of(); });
  }
  function from(links) {
      if (links.length === 0)
          return empty();
      return links.map(toLink).reduce(function (x, y) { return x.concat(y); });
  }
  function split(test, left, right) {
      var leftLink = toLink(left);
      var rightLink = toLink(right || new ApolloLink(passthrough));
      if (isTerminating(leftLink) && isTerminating(rightLink)) {
          return new ApolloLink(function (operation) {
              return test(operation)
                  ? leftLink.request(operation) || Observable.of()
                  : rightLink.request(operation) || Observable.of();
          });
      }
      else {
          return new ApolloLink(function (operation, forward) {
              return test(operation)
                  ? leftLink.request(operation, forward) || Observable.of()
                  : rightLink.request(operation, forward) || Observable.of();
          });
      }
  }
  var concat = function (first, second) {
      var firstLink = toLink(first);
      if (isTerminating(firstLink)) {
          process.env.NODE_ENV === "production" || tsInvariant.invariant.warn(new LinkError("You are calling concat on a terminating link, which will have no effect", firstLink));
          return firstLink;
      }
      var nextLink = toLink(second);
      if (isTerminating(nextLink)) {
          return new ApolloLink(function (operation) {
              return firstLink.request(operation, function (op) { return nextLink.request(op) || Observable.of(); }) || Observable.of();
          });
      }
      else {
          return new ApolloLink(function (operation, forward) {
              return (firstLink.request(operation, function (op) {
                  return nextLink.request(op, forward) || Observable.of();
              }) || Observable.of());
          });
      }
  };
  var ApolloLink = (function () {
      function ApolloLink(request) {
          if (request)
              this.request = request;
      }
      ApolloLink.prototype.split = function (test, left, right) {
          return this.concat(split(test, left, right || new ApolloLink(passthrough)));
      };
      ApolloLink.prototype.concat = function (next) {
          return concat(this, next);
      };
      ApolloLink.prototype.request = function (operation, forward) {
          throw process.env.NODE_ENV === "production" ? new tsInvariant.InvariantError(1) : new tsInvariant.InvariantError('request is not implemented');
      };
      ApolloLink.empty = empty;
      ApolloLink.from = from;
      ApolloLink.split = split;
      ApolloLink.execute = execute;
      return ApolloLink;
  }());
  function execute(link, operation) {
      return (link.request(createOperation(operation.context, transformOperation(validateOperation(operation)))) || Observable.of());
  }

  exports.Observable = Observable;
  Object.defineProperty(exports, 'getOperationName', {
    enumerable: true,
    get: function () {
      return apolloUtilities.getOperationName;
    }
  });
  exports.ApolloLink = ApolloLink;
  exports.concat = concat;
  exports.createOperation = createOperation;
  exports.empty = empty;
  exports.execute = execute;
  exports.from = from;
  exports.fromError = fromError;
  exports.fromPromise = fromPromise;
  exports.makePromise = makePromise;
  exports.split = split;
  exports.toPromise = toPromise;

  Object.defineProperty(exports, '__esModule', { value: true });

})));
//# sourceMappingURL=bundle.umd.js.map
