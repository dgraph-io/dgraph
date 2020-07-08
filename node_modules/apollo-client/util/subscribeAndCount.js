"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var observables_1 = require("./observables");
function subscribeAndCount(done, observable, cb) {
    var handleCount = 0;
    var subscription = observables_1.asyncMap(observable, function (result) {
        try {
            return cb(++handleCount, result);
        }
        catch (e) {
            setImmediate(function () {
                subscription.unsubscribe();
                done.fail(e);
            });
        }
    }).subscribe({
        error: done.fail,
    });
    return subscription;
}
exports.default = subscribeAndCount;
//# sourceMappingURL=subscribeAndCount.js.map