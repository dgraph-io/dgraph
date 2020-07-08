"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var apollo_link_1 = require("apollo-link");
var symbol_observable_1 = tslib_1.__importDefault(require("symbol-observable"));
var Observable = (function (_super) {
    tslib_1.__extends(Observable, _super);
    function Observable() {
        return _super !== null && _super.apply(this, arguments) || this;
    }
    Observable.prototype[symbol_observable_1.default] = function () {
        return this;
    };
    Observable.prototype['@@observable'] = function () {
        return this;
    };
    return Observable;
}(apollo_link_1.Observable));
exports.Observable = Observable;
//# sourceMappingURL=Observable.js.map