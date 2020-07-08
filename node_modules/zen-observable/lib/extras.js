"use strict";

Object.defineProperty(exports, "__esModule", {
  value: true
});
exports.merge = merge;
exports.combineLatest = combineLatest;
exports.zip = zip;

var _Observable = require("./Observable.js");

// Emits all values from all inputs in parallel
function merge() {
  for (var _len = arguments.length, sources = new Array(_len), _key = 0; _key < _len; _key++) {
    sources[_key] = arguments[_key];
  }

  return new _Observable.Observable(function (observer) {
    if (sources.length === 0) return _Observable.Observable.from([]);
    var count = sources.length;
    var subscriptions = sources.map(function (source) {
      return _Observable.Observable.from(source).subscribe({
        next: function (v) {
          observer.next(v);
        },
        error: function (e) {
          observer.error(e);
        },
        complete: function () {
          if (--count === 0) observer.complete();
        }
      });
    });
    return function () {
      return subscriptions.forEach(function (s) {
        return s.unsubscribe();
      });
    };
  });
} // Emits arrays containing the most current values from each input


function combineLatest() {
  for (var _len2 = arguments.length, sources = new Array(_len2), _key2 = 0; _key2 < _len2; _key2++) {
    sources[_key2] = arguments[_key2];
  }

  return new _Observable.Observable(function (observer) {
    if (sources.length === 0) return _Observable.Observable.from([]);
    var count = sources.length;
    var seen = new Set();
    var seenAll = false;
    var values = sources.map(function () {
      return undefined;
    });
    var subscriptions = sources.map(function (source, index) {
      return _Observable.Observable.from(source).subscribe({
        next: function (v) {
          values[index] = v;

          if (!seenAll) {
            seen.add(index);
            if (seen.size !== sources.length) return;
            seen = null;
            seenAll = true;
          }

          observer.next(Array.from(values));
        },
        error: function (e) {
          observer.error(e);
        },
        complete: function () {
          if (--count === 0) observer.complete();
        }
      });
    });
    return function () {
      return subscriptions.forEach(function (s) {
        return s.unsubscribe();
      });
    };
  });
} // Emits arrays containing the matching index values from each input


function zip() {
  for (var _len3 = arguments.length, sources = new Array(_len3), _key3 = 0; _key3 < _len3; _key3++) {
    sources[_key3] = arguments[_key3];
  }

  return new _Observable.Observable(function (observer) {
    if (sources.length === 0) return _Observable.Observable.from([]);
    var queues = sources.map(function () {
      return [];
    });

    function done() {
      return queues.some(function (q, i) {
        return q.length === 0 && subscriptions[i].closed;
      });
    }

    var subscriptions = sources.map(function (source, index) {
      return _Observable.Observable.from(source).subscribe({
        next: function (v) {
          queues[index].push(v);

          if (queues.every(function (q) {
            return q.length > 0;
          })) {
            observer.next(queues.map(function (q) {
              return q.shift();
            }));
            if (done()) observer.complete();
          }
        },
        error: function (e) {
          observer.error(e);
        },
        complete: function () {
          if (done()) observer.complete();
        }
      });
    });
    return function () {
      return subscriptions.forEach(function (s) {
        return s.unsubscribe();
      });
    };
  });
}