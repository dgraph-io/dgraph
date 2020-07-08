"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var link_1 = require("../link");
var MockLink = (function (_super) {
    tslib_1.__extends(MockLink, _super);
    function MockLink(handleRequest) {
        if (handleRequest === void 0) { handleRequest = function () { return null; }; }
        var _this = _super.call(this) || this;
        _this.request = handleRequest;
        return _this;
    }
    MockLink.prototype.request = function (operation, forward) {
        throw Error('should be overridden');
    };
    return MockLink;
}(link_1.ApolloLink));
exports.default = MockLink;
//# sourceMappingURL=mockLink.js.map