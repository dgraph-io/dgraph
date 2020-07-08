"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.default = (function (done, cb) { return function () {
    var args = [];
    for (var _i = 0; _i < arguments.length; _i++) {
        args[_i] = arguments[_i];
    }
    try {
        return cb.apply(void 0, args);
    }
    catch (e) {
        done.fail(e);
    }
}; });
function withWarning(func, regex) {
    var message = null;
    var oldWarn = console.warn;
    console.warn = function (m) { return (message = m); };
    return Promise.resolve(func()).then(function (val) {
        expect(message).toMatch(regex);
        console.warn = oldWarn;
        return val;
    });
}
exports.withWarning = withWarning;
function withError(func, regex) {
    var message = null;
    var oldError = console.error;
    console.error = function (m) { return (message = m); };
    try {
        var result = func();
        expect(message).toMatch(regex);
        return result;
    }
    finally {
        console.error = oldError;
    }
}
exports.withError = withError;
//# sourceMappingURL=wrap.js.map