"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var graphql_tag_1 = tslib_1.__importDefault(require("graphql-tag"));
var link_1 = require("../link");
var sampleQuery = graphql_tag_1.default(templateObject_1 || (templateObject_1 = tslib_1.__makeTemplateObject(["\n  query SampleQuery {\n    stub {\n      id\n    }\n  }\n"], ["\n  query SampleQuery {\n    stub {\n      id\n    }\n  }\n"])));
function checkCalls(calls, results) {
    if (calls === void 0) { calls = []; }
    expect(calls.length).toBe(results.length);
    calls.map(function (call, i) { return expect(call.data).toEqual(results[i]); });
}
exports.checkCalls = checkCalls;
function testLinkResults(params) {
    var link = params.link, context = params.context, variables = params.variables;
    var results = params.results || [];
    var query = params.query || sampleQuery;
    var done = params.done || (function () { return void 0; });
    var spy = jest.fn();
    link_1.execute(link, { query: query, context: context, variables: variables }).subscribe({
        next: spy,
        error: function (error) {
            expect(error).toEqual(results.pop());
            checkCalls(spy.mock.calls[0], results);
            if (done) {
                done();
            }
        },
        complete: function () {
            checkCalls(spy.mock.calls[0], results);
            if (done) {
                done();
            }
        },
    });
}
exports.testLinkResults = testLinkResults;
var templateObject_1;
//# sourceMappingURL=testingUtils.js.map