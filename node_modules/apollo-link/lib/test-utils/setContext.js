"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var link_1 = require("../link");
var SetContextLink = (function (_super) {
    tslib_1.__extends(SetContextLink, _super);
    function SetContextLink(setContext) {
        if (setContext === void 0) { setContext = function (c) { return c; }; }
        var _this = _super.call(this) || this;
        _this.setContext = setContext;
        return _this;
    }
    SetContextLink.prototype.request = function (operation, forward) {
        operation.setContext(this.setContext(operation.getContext()));
        return forward(operation);
    };
    return SetContextLink;
}(link_1.ApolloLink));
exports.default = SetContextLink;
//# sourceMappingURL=setContext.js.map