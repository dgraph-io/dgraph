(function (global, factory) {
  typeof exports === 'object' && typeof module !== 'undefined' ? factory(exports, require('zen-observable')) :
  typeof define === 'function' && define.amd ? define(['exports', 'zen-observable'], factory) :
  (global = global || self, factory((global.apolloLink = global.apolloLink || {}, global.apolloLink.zenObservable = {}), global.Observable));
}(this, (function (exports, zenObservable) { 'use strict';

  zenObservable = zenObservable && zenObservable.hasOwnProperty('default') ? zenObservable['default'] : zenObservable;

  var Observable = zenObservable;

  exports.Observable = Observable;
  exports.default = Observable;

  Object.defineProperty(exports, '__esModule', { value: true });

})));
//# sourceMappingURL=bundle.umd.js.map
