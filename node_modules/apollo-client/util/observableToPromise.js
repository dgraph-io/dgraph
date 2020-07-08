"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
function observableToPromiseAndSubscription(_a) {
    var observable = _a.observable, _b = _a.shouldResolve, shouldResolve = _b === void 0 ? true : _b, _c = _a.wait, wait = _c === void 0 ? -1 : _c, _d = _a.errorCallbacks, errorCallbacks = _d === void 0 ? [] : _d;
    var cbs = [];
    for (var _i = 1; _i < arguments.length; _i++) {
        cbs[_i - 1] = arguments[_i];
    }
    var subscription = null;
    var promise = new Promise(function (resolve, reject) {
        var errorIndex = 0;
        var cbIndex = 0;
        var results = [];
        var tryToResolve = function () {
            if (!shouldResolve) {
                return;
            }
            var done = function () {
                subscription.unsubscribe();
                resolve(results);
            };
            if (cbIndex === cbs.length && errorIndex === errorCallbacks.length) {
                if (wait === -1) {
                    done();
                }
                else {
                    setTimeout(done, wait);
                }
            }
        };
        subscription = observable.subscribe({
            next: function (result) {
                var cb = cbs[cbIndex++];
                if (cb) {
                    try {
                        results.push(cb(result));
                    }
                    catch (e) {
                        return reject(e);
                    }
                    tryToResolve();
                }
                else {
                    reject(new Error("Observable called more than " + cbs.length + " times"));
                }
            },
            error: function (error) {
                var errorCb = errorCallbacks[errorIndex++];
                if (errorCb) {
                    try {
                        errorCb(error);
                    }
                    catch (e) {
                        return reject(e);
                    }
                    tryToResolve();
                }
                else {
                    reject(error);
                }
            },
        });
    });
    return {
        promise: promise,
        subscription: subscription,
    };
}
exports.observableToPromiseAndSubscription = observableToPromiseAndSubscription;
function default_1(options) {
    var cbs = [];
    for (var _i = 1; _i < arguments.length; _i++) {
        cbs[_i - 1] = arguments[_i];
    }
    return observableToPromiseAndSubscription.apply(void 0, tslib_1.__spreadArrays([options], cbs)).promise;
}
exports.default = default_1;
//# sourceMappingURL=observableToPromise.js.map