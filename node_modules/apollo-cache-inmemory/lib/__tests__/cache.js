"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var graphql_tag_1 = tslib_1.__importStar(require("graphql-tag"));
var apollo_utilities_1 = require("apollo-utilities");
var lodash_1 = require("lodash");
var __1 = require("..");
graphql_tag_1.disableFragmentWarnings();
describe('Cache', function () {
    function itWithInitialData(message, initialDataForCaches, callback) {
        var cachesList = [
            initialDataForCaches.map(function (data) {
                return new __1.InMemoryCache({
                    addTypename: false,
                }).restore(lodash_1.cloneDeep(data));
            }),
            initialDataForCaches.map(function (data) {
                return new __1.InMemoryCache({
                    addTypename: false,
                    resultCaching: false,
                }).restore(lodash_1.cloneDeep(data));
            }),
            initialDataForCaches.map(function (data) {
                return new __1.InMemoryCache({
                    addTypename: false,
                    freezeResults: true,
                }).restore(lodash_1.cloneDeep(data));
            }),
        ];
        cachesList.forEach(function (caches, i) {
            it(message + (" (" + (i + 1) + "/" + cachesList.length + ")"), function () {
                return callback.apply(void 0, caches);
            });
        });
    }
    function itWithCacheConfig(message, config, callback) {
        var caches = [
            new __1.InMemoryCache(tslib_1.__assign(tslib_1.__assign({ addTypename: false }, config), { resultCaching: true })),
            new __1.InMemoryCache(tslib_1.__assign(tslib_1.__assign({ addTypename: false }, config), { resultCaching: false })),
            new __1.InMemoryCache(tslib_1.__assign(tslib_1.__assign({ addTypename: false }, config), { freezeResults: true })),
        ];
        caches.forEach(function (cache, i) {
            it(message + (" (" + (i + 1) + "/" + caches.length + ")"), function () { return callback(cache); });
        });
    }
    describe('readQuery', function () {
        itWithInitialData('will read some data from the store', [
            {
                ROOT_QUERY: {
                    a: 1,
                    b: 2,
                    c: 3,
                },
            },
        ], function (proxy) {
            expect(apollo_utilities_1.stripSymbols(proxy.readQuery({
                query: graphql_tag_1.default(templateObject_1 || (templateObject_1 = tslib_1.__makeTemplateObject(["\n                {\n                  a\n                }\n              "], ["\n                {\n                  a\n                }\n              "]))),
            }))).toEqual({ a: 1 });
            expect(apollo_utilities_1.stripSymbols(proxy.readQuery({
                query: graphql_tag_1.default(templateObject_2 || (templateObject_2 = tslib_1.__makeTemplateObject(["\n                {\n                  b\n                  c\n                }\n              "], ["\n                {\n                  b\n                  c\n                }\n              "]))),
            }))).toEqual({ b: 2, c: 3 });
            expect(apollo_utilities_1.stripSymbols(proxy.readQuery({
                query: graphql_tag_1.default(templateObject_3 || (templateObject_3 = tslib_1.__makeTemplateObject(["\n                {\n                  a\n                  b\n                  c\n                }\n              "], ["\n                {\n                  a\n                  b\n                  c\n                }\n              "]))),
            }))).toEqual({ a: 1, b: 2, c: 3 });
        });
        itWithInitialData('will read some deeply nested data from the store', [
            {
                ROOT_QUERY: {
                    a: 1,
                    b: 2,
                    c: 3,
                    d: {
                        type: 'id',
                        id: 'foo',
                        generated: false,
                    },
                },
                foo: {
                    e: 4,
                    f: 5,
                    g: 6,
                    h: {
                        type: 'id',
                        id: 'bar',
                        generated: false,
                    },
                },
                bar: {
                    i: 7,
                    j: 8,
                    k: 9,
                },
            },
        ], function (proxy) {
            expect(apollo_utilities_1.stripSymbols(proxy.readQuery({
                query: graphql_tag_1.default(templateObject_4 || (templateObject_4 = tslib_1.__makeTemplateObject(["\n                {\n                  a\n                  d {\n                    e\n                  }\n                }\n              "], ["\n                {\n                  a\n                  d {\n                    e\n                  }\n                }\n              "]))),
            }))).toEqual({ a: 1, d: { e: 4 } });
            expect(apollo_utilities_1.stripSymbols(proxy.readQuery({
                query: graphql_tag_1.default(templateObject_5 || (templateObject_5 = tslib_1.__makeTemplateObject(["\n                {\n                  a\n                  d {\n                    e\n                    h {\n                      i\n                    }\n                  }\n                }\n              "], ["\n                {\n                  a\n                  d {\n                    e\n                    h {\n                      i\n                    }\n                  }\n                }\n              "]))),
            }))).toEqual({ a: 1, d: { e: 4, h: { i: 7 } } });
            expect(apollo_utilities_1.stripSymbols(proxy.readQuery({
                query: graphql_tag_1.default(templateObject_6 || (templateObject_6 = tslib_1.__makeTemplateObject(["\n                {\n                  a\n                  b\n                  c\n                  d {\n                    e\n                    f\n                    g\n                    h {\n                      i\n                      j\n                      k\n                    }\n                  }\n                }\n              "], ["\n                {\n                  a\n                  b\n                  c\n                  d {\n                    e\n                    f\n                    g\n                    h {\n                      i\n                      j\n                      k\n                    }\n                  }\n                }\n              "]))),
            }))).toEqual({
                a: 1,
                b: 2,
                c: 3,
                d: { e: 4, f: 5, g: 6, h: { i: 7, j: 8, k: 9 } },
            });
        });
        itWithInitialData('will read some data from the store with variables', [
            {
                ROOT_QUERY: {
                    'field({"literal":true,"value":42})': 1,
                    'field({"literal":false,"value":42})': 2,
                },
            },
        ], function (proxy) {
            expect(apollo_utilities_1.stripSymbols(proxy.readQuery({
                query: graphql_tag_1.default(templateObject_7 || (templateObject_7 = tslib_1.__makeTemplateObject(["\n                query($literal: Boolean, $value: Int) {\n                  a: field(literal: true, value: 42)\n                  b: field(literal: $literal, value: $value)\n                }\n              "], ["\n                query($literal: Boolean, $value: Int) {\n                  a: field(literal: true, value: 42)\n                  b: field(literal: $literal, value: $value)\n                }\n              "]))),
                variables: {
                    literal: false,
                    value: 42,
                },
            }))).toEqual({ a: 1, b: 2 });
        });
        itWithInitialData('will read some data from the store with null variables', [
            {
                ROOT_QUERY: {
                    'field({"literal":false,"value":null})': 1,
                },
            },
        ], function (proxy) {
            expect(apollo_utilities_1.stripSymbols(proxy.readQuery({
                query: graphql_tag_1.default(templateObject_8 || (templateObject_8 = tslib_1.__makeTemplateObject(["\n                query($literal: Boolean, $value: Int) {\n                  a: field(literal: $literal, value: $value)\n                }\n              "], ["\n                query($literal: Boolean, $value: Int) {\n                  a: field(literal: $literal, value: $value)\n                }\n              "]))),
                variables: {
                    literal: false,
                    value: null,
                },
            }))).toEqual({ a: 1 });
        });
        itWithInitialData('should not mutate arguments passed in', [
            {
                ROOT_QUERY: {
                    'field({"literal":true,"value":42})': 1,
                    'field({"literal":false,"value":42})': 2,
                },
            },
        ], function (proxy) {
            var options = {
                query: graphql_tag_1.default(templateObject_9 || (templateObject_9 = tslib_1.__makeTemplateObject(["\n            query($literal: Boolean, $value: Int) {\n              a: field(literal: true, value: 42)\n              b: field(literal: $literal, value: $value)\n            }\n          "], ["\n            query($literal: Boolean, $value: Int) {\n              a: field(literal: true, value: 42)\n              b: field(literal: $literal, value: $value)\n            }\n          "]))),
                variables: {
                    literal: false,
                    value: 42,
                },
            };
            var preQueryCopy = lodash_1.cloneDeep(options);
            expect(apollo_utilities_1.stripSymbols(proxy.readQuery(options))).toEqual({ a: 1, b: 2 });
            expect(preQueryCopy).toEqual(options);
        });
    });
    describe('readFragment', function () {
        itWithInitialData('will throw an error when there is no fragment', [
            {},
        ], function (proxy) {
            expect(function () {
                proxy.readFragment({
                    id: 'x',
                    fragment: graphql_tag_1.default(templateObject_10 || (templateObject_10 = tslib_1.__makeTemplateObject(["\n              query {\n                a\n                b\n                c\n              }\n            "], ["\n              query {\n                a\n                b\n                c\n              }\n            "]))),
                });
            }).toThrowError('Found a query operation. No operations are allowed when using a fragment as a query. Only fragments are allowed.');
            expect(function () {
                proxy.readFragment({
                    id: 'x',
                    fragment: graphql_tag_1.default(templateObject_11 || (templateObject_11 = tslib_1.__makeTemplateObject(["\n              schema {\n                query: Query\n              }\n            "], ["\n              schema {\n                query: Query\n              }\n            "]))),
                });
            }).toThrowError('Found 0 fragments. `fragmentName` must be provided when there is not exactly 1 fragment.');
        });
        itWithInitialData('will throw an error when there is more than one fragment but no fragment name', [{}], function (proxy) {
            expect(function () {
                proxy.readFragment({
                    id: 'x',
                    fragment: graphql_tag_1.default(templateObject_12 || (templateObject_12 = tslib_1.__makeTemplateObject(["\n              fragment a on A {\n                a\n              }\n\n              fragment b on B {\n                b\n              }\n            "], ["\n              fragment a on A {\n                a\n              }\n\n              fragment b on B {\n                b\n              }\n            "]))),
                });
            }).toThrowError('Found 2 fragments. `fragmentName` must be provided when there is not exactly 1 fragment.');
            expect(function () {
                proxy.readFragment({
                    id: 'x',
                    fragment: graphql_tag_1.default(templateObject_13 || (templateObject_13 = tslib_1.__makeTemplateObject(["\n              fragment a on A {\n                a\n              }\n\n              fragment b on B {\n                b\n              }\n\n              fragment c on C {\n                c\n              }\n            "], ["\n              fragment a on A {\n                a\n              }\n\n              fragment b on B {\n                b\n              }\n\n              fragment c on C {\n                c\n              }\n            "]))),
                });
            }).toThrowError('Found 3 fragments. `fragmentName` must be provided when there is not exactly 1 fragment.');
        });
        itWithInitialData('will read some deeply nested data from the store at any id', [
            {
                ROOT_QUERY: {
                    __typename: 'Type1',
                    a: 1,
                    b: 2,
                    c: 3,
                    d: {
                        type: 'id',
                        id: 'foo',
                        generated: false,
                    },
                },
                foo: {
                    __typename: 'Foo',
                    e: 4,
                    f: 5,
                    g: 6,
                    h: {
                        type: 'id',
                        id: 'bar',
                        generated: false,
                    },
                },
                bar: {
                    __typename: 'Bar',
                    i: 7,
                    j: 8,
                    k: 9,
                },
            },
        ], function (proxy) {
            expect(apollo_utilities_1.stripSymbols(proxy.readFragment({
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_14 || (templateObject_14 = tslib_1.__makeTemplateObject(["\n                fragment fragmentFoo on Foo {\n                  e\n                  h {\n                    i\n                  }\n                }\n              "], ["\n                fragment fragmentFoo on Foo {\n                  e\n                  h {\n                    i\n                  }\n                }\n              "]))),
            }))).toEqual({ e: 4, h: { i: 7 } });
            expect(apollo_utilities_1.stripSymbols(proxy.readFragment({
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_15 || (templateObject_15 = tslib_1.__makeTemplateObject(["\n                fragment fragmentFoo on Foo {\n                  e\n                  f\n                  g\n                  h {\n                    i\n                    j\n                    k\n                  }\n                }\n              "], ["\n                fragment fragmentFoo on Foo {\n                  e\n                  f\n                  g\n                  h {\n                    i\n                    j\n                    k\n                  }\n                }\n              "]))),
            }))).toEqual({ e: 4, f: 5, g: 6, h: { i: 7, j: 8, k: 9 } });
            expect(apollo_utilities_1.stripSymbols(proxy.readFragment({
                id: 'bar',
                fragment: graphql_tag_1.default(templateObject_16 || (templateObject_16 = tslib_1.__makeTemplateObject(["\n                fragment fragmentBar on Bar {\n                  i\n                }\n              "], ["\n                fragment fragmentBar on Bar {\n                  i\n                }\n              "]))),
            }))).toEqual({ i: 7 });
            expect(apollo_utilities_1.stripSymbols(proxy.readFragment({
                id: 'bar',
                fragment: graphql_tag_1.default(templateObject_17 || (templateObject_17 = tslib_1.__makeTemplateObject(["\n                fragment fragmentBar on Bar {\n                  i\n                  j\n                  k\n                }\n              "], ["\n                fragment fragmentBar on Bar {\n                  i\n                  j\n                  k\n                }\n              "]))),
            }))).toEqual({ i: 7, j: 8, k: 9 });
            expect(apollo_utilities_1.stripSymbols(proxy.readFragment({
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_18 || (templateObject_18 = tslib_1.__makeTemplateObject(["\n                fragment fragmentFoo on Foo {\n                  e\n                  f\n                  g\n                  h {\n                    i\n                    j\n                    k\n                  }\n                }\n\n                fragment fragmentBar on Bar {\n                  i\n                  j\n                  k\n                }\n              "], ["\n                fragment fragmentFoo on Foo {\n                  e\n                  f\n                  g\n                  h {\n                    i\n                    j\n                    k\n                  }\n                }\n\n                fragment fragmentBar on Bar {\n                  i\n                  j\n                  k\n                }\n              "]))),
                fragmentName: 'fragmentFoo',
            }))).toEqual({ e: 4, f: 5, g: 6, h: { i: 7, j: 8, k: 9 } });
            expect(apollo_utilities_1.stripSymbols(proxy.readFragment({
                id: 'bar',
                fragment: graphql_tag_1.default(templateObject_19 || (templateObject_19 = tslib_1.__makeTemplateObject(["\n                fragment fragmentFoo on Foo {\n                  e\n                  f\n                  g\n                  h {\n                    i\n                    j\n                    k\n                  }\n                }\n\n                fragment fragmentBar on Bar {\n                  i\n                  j\n                  k\n                }\n              "], ["\n                fragment fragmentFoo on Foo {\n                  e\n                  f\n                  g\n                  h {\n                    i\n                    j\n                    k\n                  }\n                }\n\n                fragment fragmentBar on Bar {\n                  i\n                  j\n                  k\n                }\n              "]))),
                fragmentName: 'fragmentBar',
            }))).toEqual({ i: 7, j: 8, k: 9 });
        });
        itWithInitialData('will read some data from the store with variables', [
            {
                foo: {
                    __typename: 'Foo',
                    'field({"literal":true,"value":42})': 1,
                    'field({"literal":false,"value":42})': 2,
                },
            },
        ], function (proxy) {
            expect(apollo_utilities_1.stripSymbols(proxy.readFragment({
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_20 || (templateObject_20 = tslib_1.__makeTemplateObject(["\n                fragment foo on Foo {\n                  a: field(literal: true, value: 42)\n                  b: field(literal: $literal, value: $value)\n                }\n              "], ["\n                fragment foo on Foo {\n                  a: field(literal: true, value: 42)\n                  b: field(literal: $literal, value: $value)\n                }\n              "]))),
                variables: {
                    literal: false,
                    value: 42,
                },
            }))).toEqual({ a: 1, b: 2 });
        });
        itWithInitialData('will return null when an id that can’t be found is provided', [
            {},
            {
                bar: { __typename: 'Bar', a: 1, b: 2, c: 3 },
            },
            {
                foo: { __typename: 'Foo', a: 1, b: 2, c: 3 },
            },
        ], function (client1, client2, client3) {
            expect(apollo_utilities_1.stripSymbols(client1.readFragment({
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_21 || (templateObject_21 = tslib_1.__makeTemplateObject(["\n                fragment fooFragment on Foo {\n                  a\n                  b\n                  c\n                }\n              "], ["\n                fragment fooFragment on Foo {\n                  a\n                  b\n                  c\n                }\n              "]))),
            }))).toEqual(null);
            expect(apollo_utilities_1.stripSymbols(client2.readFragment({
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_22 || (templateObject_22 = tslib_1.__makeTemplateObject(["\n                fragment fooFragment on Foo {\n                  a\n                  b\n                  c\n                }\n              "], ["\n                fragment fooFragment on Foo {\n                  a\n                  b\n                  c\n                }\n              "]))),
            }))).toEqual(null);
            expect(apollo_utilities_1.stripSymbols(client3.readFragment({
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_23 || (templateObject_23 = tslib_1.__makeTemplateObject(["\n                fragment fooFragment on Foo {\n                  a\n                  b\n                  c\n                }\n              "], ["\n                fragment fooFragment on Foo {\n                  a\n                  b\n                  c\n                }\n              "]))),
            }))).toEqual({ a: 1, b: 2, c: 3 });
        });
    });
    describe('writeQuery', function () {
        itWithInitialData('will write some data to the store', [{}], function (proxy) {
            proxy.writeQuery({
                data: { a: 1 },
                query: graphql_tag_1.default(templateObject_24 || (templateObject_24 = tslib_1.__makeTemplateObject(["\n          {\n            a\n          }\n        "], ["\n          {\n            a\n          }\n        "]))),
            });
            expect(proxy.extract()).toEqual({
                ROOT_QUERY: {
                    a: 1,
                },
            });
            proxy.writeQuery({
                data: { b: 2, c: 3 },
                query: graphql_tag_1.default(templateObject_25 || (templateObject_25 = tslib_1.__makeTemplateObject(["\n          {\n            b\n            c\n          }\n        "], ["\n          {\n            b\n            c\n          }\n        "]))),
            });
            expect(proxy.extract()).toEqual({
                ROOT_QUERY: {
                    a: 1,
                    b: 2,
                    c: 3,
                },
            });
            proxy.writeQuery({
                data: { a: 4, b: 5, c: 6 },
                query: graphql_tag_1.default(templateObject_26 || (templateObject_26 = tslib_1.__makeTemplateObject(["\n          {\n            a\n            b\n            c\n          }\n        "], ["\n          {\n            a\n            b\n            c\n          }\n        "]))),
            });
            expect(proxy.extract()).toEqual({
                ROOT_QUERY: {
                    a: 4,
                    b: 5,
                    c: 6,
                },
            });
        });
        itWithInitialData('will write some deeply nested data to the store', [{}], function (proxy) {
            proxy.writeQuery({
                data: { a: 1, d: { e: 4 } },
                query: graphql_tag_1.default(templateObject_27 || (templateObject_27 = tslib_1.__makeTemplateObject(["\n            {\n              a\n              d {\n                e\n              }\n            }\n          "], ["\n            {\n              a\n              d {\n                e\n              }\n            }\n          "]))),
            });
            expect(proxy.extract()).toEqual({
                ROOT_QUERY: {
                    a: 1,
                    d: {
                        type: 'id',
                        id: '$ROOT_QUERY.d',
                        generated: true,
                    },
                },
                '$ROOT_QUERY.d': {
                    e: 4,
                },
            });
            proxy.writeQuery({
                data: { a: 1, d: { h: { i: 7 } } },
                query: graphql_tag_1.default(templateObject_28 || (templateObject_28 = tslib_1.__makeTemplateObject(["\n            {\n              a\n              d {\n                h {\n                  i\n                }\n              }\n            }\n          "], ["\n            {\n              a\n              d {\n                h {\n                  i\n                }\n              }\n            }\n          "]))),
            });
            expect(proxy.extract()).toEqual({
                ROOT_QUERY: {
                    a: 1,
                    d: {
                        type: 'id',
                        id: '$ROOT_QUERY.d',
                        generated: true,
                    },
                },
                '$ROOT_QUERY.d': {
                    e: 4,
                    h: {
                        type: 'id',
                        id: '$ROOT_QUERY.d.h',
                        generated: true,
                    },
                },
                '$ROOT_QUERY.d.h': {
                    i: 7,
                },
            });
            proxy.writeQuery({
                data: {
                    a: 1,
                    b: 2,
                    c: 3,
                    d: { e: 4, f: 5, g: 6, h: { i: 7, j: 8, k: 9 } },
                },
                query: graphql_tag_1.default(templateObject_29 || (templateObject_29 = tslib_1.__makeTemplateObject(["\n            {\n              a\n              b\n              c\n              d {\n                e\n                f\n                g\n                h {\n                  i\n                  j\n                  k\n                }\n              }\n            }\n          "], ["\n            {\n              a\n              b\n              c\n              d {\n                e\n                f\n                g\n                h {\n                  i\n                  j\n                  k\n                }\n              }\n            }\n          "]))),
            });
            expect(proxy.extract()).toEqual({
                ROOT_QUERY: {
                    a: 1,
                    b: 2,
                    c: 3,
                    d: {
                        type: 'id',
                        id: '$ROOT_QUERY.d',
                        generated: true,
                    },
                },
                '$ROOT_QUERY.d': {
                    e: 4,
                    f: 5,
                    g: 6,
                    h: {
                        type: 'id',
                        id: '$ROOT_QUERY.d.h',
                        generated: true,
                    },
                },
                '$ROOT_QUERY.d.h': {
                    i: 7,
                    j: 8,
                    k: 9,
                },
            });
        });
        itWithInitialData('will write some data to the store with variables', [{}], function (proxy) {
            proxy.writeQuery({
                data: {
                    a: 1,
                    b: 2,
                },
                query: graphql_tag_1.default(templateObject_30 || (templateObject_30 = tslib_1.__makeTemplateObject(["\n            query($literal: Boolean, $value: Int) {\n              a: field(literal: true, value: 42)\n              b: field(literal: $literal, value: $value)\n            }\n          "], ["\n            query($literal: Boolean, $value: Int) {\n              a: field(literal: true, value: 42)\n              b: field(literal: $literal, value: $value)\n            }\n          "]))),
                variables: {
                    literal: false,
                    value: 42,
                },
            });
            expect(proxy.extract()).toEqual({
                ROOT_QUERY: {
                    'field({"literal":true,"value":42})': 1,
                    'field({"literal":false,"value":42})': 2,
                },
            });
        });
        itWithInitialData('will write some data to the store with variables where some are null', [{}], function (proxy) {
            proxy.writeQuery({
                data: {
                    a: 1,
                    b: 2,
                },
                query: graphql_tag_1.default(templateObject_31 || (templateObject_31 = tslib_1.__makeTemplateObject(["\n            query($literal: Boolean, $value: Int) {\n              a: field(literal: true, value: 42)\n              b: field(literal: $literal, value: $value)\n            }\n          "], ["\n            query($literal: Boolean, $value: Int) {\n              a: field(literal: true, value: 42)\n              b: field(literal: $literal, value: $value)\n            }\n          "]))),
                variables: {
                    literal: false,
                    value: null,
                },
            });
            expect(proxy.extract()).toEqual({
                ROOT_QUERY: {
                    'field({"literal":true,"value":42})': 1,
                    'field({"literal":false,"value":null})': 2,
                },
            });
        });
    });
    describe('writeFragment', function () {
        itWithInitialData('will throw an error when there is no fragment', [{}], function (proxy) {
            expect(function () {
                proxy.writeFragment({
                    data: {},
                    id: 'x',
                    fragment: graphql_tag_1.default(templateObject_32 || (templateObject_32 = tslib_1.__makeTemplateObject(["\n              query {\n                a\n                b\n                c\n              }\n            "], ["\n              query {\n                a\n                b\n                c\n              }\n            "]))),
                });
            }).toThrowError('Found a query operation. No operations are allowed when using a fragment as a query. Only fragments are allowed.');
            expect(function () {
                proxy.writeFragment({
                    data: {},
                    id: 'x',
                    fragment: graphql_tag_1.default(templateObject_33 || (templateObject_33 = tslib_1.__makeTemplateObject(["\n              schema {\n                query: Query\n              }\n            "], ["\n              schema {\n                query: Query\n              }\n            "]))),
                });
            }).toThrowError('Found 0 fragments. `fragmentName` must be provided when there is not exactly 1 fragment.');
        });
        itWithInitialData('will throw an error when there is more than one fragment but no fragment name', [{}], function (proxy) {
            expect(function () {
                proxy.writeFragment({
                    data: {},
                    id: 'x',
                    fragment: graphql_tag_1.default(templateObject_34 || (templateObject_34 = tslib_1.__makeTemplateObject(["\n              fragment a on A {\n                a\n              }\n\n              fragment b on B {\n                b\n              }\n            "], ["\n              fragment a on A {\n                a\n              }\n\n              fragment b on B {\n                b\n              }\n            "]))),
                });
            }).toThrowError('Found 2 fragments. `fragmentName` must be provided when there is not exactly 1 fragment.');
            expect(function () {
                proxy.writeFragment({
                    data: {},
                    id: 'x',
                    fragment: graphql_tag_1.default(templateObject_35 || (templateObject_35 = tslib_1.__makeTemplateObject(["\n              fragment a on A {\n                a\n              }\n\n              fragment b on B {\n                b\n              }\n\n              fragment c on C {\n                c\n              }\n            "], ["\n              fragment a on A {\n                a\n              }\n\n              fragment b on B {\n                b\n              }\n\n              fragment c on C {\n                c\n              }\n            "]))),
                });
            }).toThrowError('Found 3 fragments. `fragmentName` must be provided when there is not exactly 1 fragment.');
        });
        itWithCacheConfig('will write some deeply nested data into the store at any id', {
            dataIdFromObject: function (o) { return o.id; },
            addTypename: false,
        }, function (proxy) {
            proxy.writeFragment({
                data: { __typename: 'Foo', e: 4, h: { id: 'bar', i: 7 } },
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_36 || (templateObject_36 = tslib_1.__makeTemplateObject(["\n            fragment fragmentFoo on Foo {\n              e\n              h {\n                i\n              }\n            }\n          "], ["\n            fragment fragmentFoo on Foo {\n              e\n              h {\n                i\n              }\n            }\n          "]))),
            });
            expect(proxy.extract()).toMatchSnapshot();
            proxy.writeFragment({
                data: { __typename: 'Foo', f: 5, g: 6, h: { id: 'bar', j: 8, k: 9 } },
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_37 || (templateObject_37 = tslib_1.__makeTemplateObject(["\n            fragment fragmentFoo on Foo {\n              f\n              g\n              h {\n                j\n                k\n              }\n            }\n          "], ["\n            fragment fragmentFoo on Foo {\n              f\n              g\n              h {\n                j\n                k\n              }\n            }\n          "]))),
            });
            expect(proxy.extract()).toMatchSnapshot();
            proxy.writeFragment({
                data: { i: 10, __typename: 'Bar' },
                id: 'bar',
                fragment: graphql_tag_1.default(templateObject_38 || (templateObject_38 = tslib_1.__makeTemplateObject(["\n            fragment fragmentBar on Bar {\n              i\n            }\n          "], ["\n            fragment fragmentBar on Bar {\n              i\n            }\n          "]))),
            });
            expect(proxy.extract()).toMatchSnapshot();
            proxy.writeFragment({
                data: { j: 11, k: 12, __typename: 'Bar' },
                id: 'bar',
                fragment: graphql_tag_1.default(templateObject_39 || (templateObject_39 = tslib_1.__makeTemplateObject(["\n            fragment fragmentBar on Bar {\n              j\n              k\n            }\n          "], ["\n            fragment fragmentBar on Bar {\n              j\n              k\n            }\n          "]))),
            });
            expect(proxy.extract()).toMatchSnapshot();
            proxy.writeFragment({
                data: {
                    __typename: 'Foo',
                    e: 4,
                    f: 5,
                    g: 6,
                    h: { __typename: 'Bar', id: 'bar', i: 7, j: 8, k: 9 },
                },
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_40 || (templateObject_40 = tslib_1.__makeTemplateObject(["\n            fragment fooFragment on Foo {\n              e\n              f\n              g\n              h {\n                i\n                j\n                k\n              }\n            }\n\n            fragment barFragment on Bar {\n              i\n              j\n              k\n            }\n          "], ["\n            fragment fooFragment on Foo {\n              e\n              f\n              g\n              h {\n                i\n                j\n                k\n              }\n            }\n\n            fragment barFragment on Bar {\n              i\n              j\n              k\n            }\n          "]))),
                fragmentName: 'fooFragment',
            });
            expect(proxy.extract()).toMatchSnapshot();
            proxy.writeFragment({
                data: { __typename: 'Bar', i: 10, j: 11, k: 12 },
                id: 'bar',
                fragment: graphql_tag_1.default(templateObject_41 || (templateObject_41 = tslib_1.__makeTemplateObject(["\n            fragment fooFragment on Foo {\n              e\n              f\n              g\n              h {\n                i\n                j\n                k\n              }\n            }\n\n            fragment barFragment on Bar {\n              i\n              j\n              k\n            }\n          "], ["\n            fragment fooFragment on Foo {\n              e\n              f\n              g\n              h {\n                i\n                j\n                k\n              }\n            }\n\n            fragment barFragment on Bar {\n              i\n              j\n              k\n            }\n          "]))),
                fragmentName: 'barFragment',
            });
            expect(proxy.extract()).toMatchSnapshot();
        });
        itWithCacheConfig('writes data that can be read back', {
            addTypename: true,
        }, function (proxy) {
            var readWriteFragment = graphql_tag_1.default(templateObject_42 || (templateObject_42 = tslib_1.__makeTemplateObject(["\n          fragment aFragment on query {\n            getSomething {\n              id\n            }\n          }\n        "], ["\n          fragment aFragment on query {\n            getSomething {\n              id\n            }\n          }\n        "])));
            var data = {
                __typename: 'query',
                getSomething: { id: '123', __typename: 'Something' },
            };
            proxy.writeFragment({
                data: data,
                id: 'query',
                fragment: readWriteFragment,
            });
            var result = proxy.readFragment({
                fragment: readWriteFragment,
                id: 'query',
            });
            expect(apollo_utilities_1.stripSymbols(result)).toEqual(data);
        });
        itWithCacheConfig('will write some data to the store with variables', {
            addTypename: true,
        }, function (proxy) {
            proxy.writeFragment({
                data: {
                    a: 1,
                    b: 2,
                    __typename: 'Foo',
                },
                id: 'foo',
                fragment: graphql_tag_1.default(templateObject_43 || (templateObject_43 = tslib_1.__makeTemplateObject(["\n            fragment foo on Foo {\n              a: field(literal: true, value: 42)\n              b: field(literal: $literal, value: $value)\n            }\n          "], ["\n            fragment foo on Foo {\n              a: field(literal: true, value: 42)\n              b: field(literal: $literal, value: $value)\n            }\n          "]))),
                variables: {
                    literal: false,
                    value: 42,
                },
            });
            expect(proxy.extract()).toEqual({
                foo: {
                    __typename: 'Foo',
                    'field({"literal":true,"value":42})': 1,
                    'field({"literal":false,"value":42})': 2,
                },
            });
        });
    });
    describe('performTransaction', function () {
        itWithInitialData('will not broadcast mid-transaction', [{}], function (cache) {
            var numBroadcasts = 0;
            var query = graphql_tag_1.default(templateObject_44 || (templateObject_44 = tslib_1.__makeTemplateObject(["\n        {\n          a\n        }\n      "], ["\n        {\n          a\n        }\n      "])));
            cache.watch({
                query: query,
                optimistic: false,
                callback: function () {
                    numBroadcasts++;
                },
            });
            expect(numBroadcasts).toEqual(0);
            cache.performTransaction(function (proxy) {
                proxy.writeQuery({
                    data: { a: 1 },
                    query: query,
                });
                expect(numBroadcasts).toEqual(0);
                proxy.writeQuery({
                    data: { a: 4, b: 5, c: 6 },
                    query: graphql_tag_1.default(templateObject_45 || (templateObject_45 = tslib_1.__makeTemplateObject(["\n            {\n              a\n              b\n              c\n            }\n          "], ["\n            {\n              a\n              b\n              c\n            }\n          "]))),
                });
                expect(numBroadcasts).toEqual(0);
            });
            expect(numBroadcasts).toEqual(1);
        });
    });
    describe('performOptimisticTransaction', function () {
        itWithInitialData('will only broadcast once', [{}], function (cache) {
            var numBroadcasts = 0;
            var query = graphql_tag_1.default(templateObject_46 || (templateObject_46 = tslib_1.__makeTemplateObject(["\n        {\n          a\n        }\n      "], ["\n        {\n          a\n        }\n      "])));
            cache.watch({
                query: query,
                optimistic: true,
                callback: function () {
                    numBroadcasts++;
                },
            });
            expect(numBroadcasts).toEqual(0);
            cache.recordOptimisticTransaction(function (proxy) {
                proxy.writeQuery({
                    data: { a: 1 },
                    query: query,
                });
                expect(numBroadcasts).toEqual(0);
                proxy.writeQuery({
                    data: { a: 4, b: 5, c: 6 },
                    query: graphql_tag_1.default(templateObject_47 || (templateObject_47 = tslib_1.__makeTemplateObject(["\n              {\n                a\n                b\n                c\n              }\n            "], ["\n              {\n                a\n                b\n                c\n              }\n            "]))),
                });
                expect(numBroadcasts).toEqual(0);
            }, 1);
            expect(numBroadcasts).toEqual(1);
        });
    });
});
var templateObject_1, templateObject_2, templateObject_3, templateObject_4, templateObject_5, templateObject_6, templateObject_7, templateObject_8, templateObject_9, templateObject_10, templateObject_11, templateObject_12, templateObject_13, templateObject_14, templateObject_15, templateObject_16, templateObject_17, templateObject_18, templateObject_19, templateObject_20, templateObject_21, templateObject_22, templateObject_23, templateObject_24, templateObject_25, templateObject_26, templateObject_27, templateObject_28, templateObject_29, templateObject_30, templateObject_31, templateObject_32, templateObject_33, templateObject_34, templateObject_35, templateObject_36, templateObject_37, templateObject_38, templateObject_39, templateObject_40, templateObject_41, templateObject_42, templateObject_43, templateObject_44, templateObject_45, templateObject_46, templateObject_47;
//# sourceMappingURL=cache.js.map