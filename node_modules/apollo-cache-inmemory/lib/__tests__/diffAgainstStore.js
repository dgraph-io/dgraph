"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var graphql_tag_1 = tslib_1.__importStar(require("graphql-tag"));
var apollo_utilities_1 = require("apollo-utilities");
var objectCache_1 = require("../objectCache");
var readFromStore_1 = require("../readFromStore");
var writeToStore_1 = require("../writeToStore");
var fragmentMatcher_1 = require("../fragmentMatcher");
var inMemoryCache_1 = require("../inMemoryCache");
var fragmentMatcherFunction = new fragmentMatcher_1.HeuristicFragmentMatcher().match;
graphql_tag_1.disableFragmentWarnings();
function withError(func, regex) {
    var message = null;
    var error = console.error;
    console.error = function (m) {
        message = m;
    };
    try {
        var result = func();
        expect(message).toMatch(regex);
        return result;
    }
    finally {
        console.error = error;
    }
}
exports.withError = withError;
describe('diffing queries against the store', function () {
    var reader = new readFromStore_1.StoreReader();
    var writer = new writeToStore_1.StoreWriter();
    it('expects named fragments to return complete as true when diffd against ' +
        'the store', function () {
        var store = objectCache_1.defaultNormalizedCacheFactory({});
        var queryResult = reader.diffQueryAgainstStore({
            store: store,
            query: graphql_tag_1.default(templateObject_1 || (templateObject_1 = tslib_1.__makeTemplateObject(["\n          query foo {\n            ...root\n          }\n\n          fragment root on Query {\n            nestedObj {\n              innerArray {\n                id\n                someField\n              }\n            }\n          }\n        "], ["\n          query foo {\n            ...root\n          }\n\n          fragment root on Query {\n            nestedObj {\n              innerArray {\n                id\n                someField\n              }\n            }\n          }\n        "]))),
            fragmentMatcherFunction: fragmentMatcherFunction,
            config: {
                dataIdFromObject: inMemoryCache_1.defaultDataIdFromObject,
            },
        });
        expect(queryResult.complete).toEqual(false);
    });
    it('expects inline fragments to return complete as true when diffd against ' +
        'the store', function () {
        var store = objectCache_1.defaultNormalizedCacheFactory();
        var queryResult = reader.diffQueryAgainstStore({
            store: store,
            query: graphql_tag_1.default(templateObject_2 || (templateObject_2 = tslib_1.__makeTemplateObject(["\n          {\n            ... on DummyQuery {\n              nestedObj {\n                innerArray {\n                  id\n                  otherField\n                }\n              }\n            }\n            ... on Query {\n              nestedObj {\n                innerArray {\n                  id\n                  someField\n                }\n              }\n            }\n            ... on DummyQuery2 {\n              nestedObj {\n                innerArray {\n                  id\n                  otherField2\n                }\n              }\n            }\n          }\n        "], ["\n          {\n            ... on DummyQuery {\n              nestedObj {\n                innerArray {\n                  id\n                  otherField\n                }\n              }\n            }\n            ... on Query {\n              nestedObj {\n                innerArray {\n                  id\n                  someField\n                }\n              }\n            }\n            ... on DummyQuery2 {\n              nestedObj {\n                innerArray {\n                  id\n                  otherField2\n                }\n              }\n            }\n          }\n        "]))),
            fragmentMatcherFunction: fragmentMatcherFunction,
            config: {
                dataIdFromObject: inMemoryCache_1.defaultDataIdFromObject,
            },
        });
        expect(queryResult.complete).toEqual(false);
    });
    it('returns nothing when the store is enough', function () {
        var query = graphql_tag_1.default(templateObject_3 || (templateObject_3 = tslib_1.__makeTemplateObject(["\n      {\n        people_one(id: \"1\") {\n          name\n        }\n      }\n    "], ["\n      {\n        people_one(id: \"1\") {\n          name\n        }\n      }\n    "])));
        var result = {
            people_one: {
                name: 'Luke Skywalker',
            },
        };
        var store = writer.writeQueryToStore({
            result: result,
            query: query,
        });
        expect(reader.diffQueryAgainstStore({
            store: store,
            query: query,
        }).complete).toBeTruthy();
    });
    it('caches root queries both under the ID of the node and the query name', function () {
        var firstQuery = graphql_tag_1.default(templateObject_4 || (templateObject_4 = tslib_1.__makeTemplateObject(["\n      {\n        people_one(id: \"1\") {\n          __typename\n          id\n          name\n        }\n      }\n    "], ["\n      {\n        people_one(id: \"1\") {\n          __typename\n          id\n          name\n        }\n      }\n    "])));
        var result = {
            people_one: {
                __typename: 'Person',
                id: '1',
                name: 'Luke Skywalker',
            },
        };
        var getIdField = function (_a) {
            var id = _a.id;
            return id;
        };
        var store = writer.writeQueryToStore({
            result: result,
            query: firstQuery,
            dataIdFromObject: getIdField,
        });
        var secondQuery = graphql_tag_1.default(templateObject_5 || (templateObject_5 = tslib_1.__makeTemplateObject(["\n      {\n        people_one(id: \"1\") {\n          __typename\n          id\n          name\n        }\n      }\n    "], ["\n      {\n        people_one(id: \"1\") {\n          __typename\n          id\n          name\n        }\n      }\n    "])));
        var complete = reader.diffQueryAgainstStore({
            store: store,
            query: secondQuery,
        }).complete;
        expect(complete).toBeTruthy();
        expect(store.get('1')).toEqual(result.people_one);
    });
    it('does not swallow errors other than field errors', function () {
        var firstQuery = graphql_tag_1.default(templateObject_6 || (templateObject_6 = tslib_1.__makeTemplateObject(["\n      query {\n        person {\n          powers\n        }\n      }\n    "], ["\n      query {\n        person {\n          powers\n        }\n      }\n    "])));
        var firstResult = {
            person: {
                powers: 'the force',
            },
        };
        var store = writer.writeQueryToStore({
            result: firstResult,
            query: firstQuery,
        });
        var unionQuery = graphql_tag_1.default(templateObject_7 || (templateObject_7 = tslib_1.__makeTemplateObject(["\n      query {\n        ...notARealFragment\n      }\n    "], ["\n      query {\n        ...notARealFragment\n      }\n    "])));
        return expect(function () {
            reader.diffQueryAgainstStore({
                store: store,
                query: unionQuery,
            });
        }).toThrowError(/No fragment/);
    });
    it('does not error on a correct query with union typed fragments', function () {
        return withError(function () {
            var firstQuery = graphql_tag_1.default(templateObject_8 || (templateObject_8 = tslib_1.__makeTemplateObject(["\n        query {\n          person {\n            __typename\n            firstName\n            lastName\n          }\n        }\n      "], ["\n        query {\n          person {\n            __typename\n            firstName\n            lastName\n          }\n        }\n      "])));
            var firstResult = {
                person: {
                    __typename: 'Author',
                    firstName: 'John',
                    lastName: 'Smith',
                },
            };
            var store = writer.writeQueryToStore({
                result: firstResult,
                query: firstQuery,
            });
            var unionQuery = graphql_tag_1.default(templateObject_9 || (templateObject_9 = tslib_1.__makeTemplateObject(["\n        query {\n          person {\n            __typename\n            ... on Author {\n              firstName\n              lastName\n            }\n            ... on Jedi {\n              powers\n            }\n          }\n        }\n      "], ["\n        query {\n          person {\n            __typename\n            ... on Author {\n              firstName\n              lastName\n            }\n            ... on Jedi {\n              powers\n            }\n          }\n        }\n      "])));
            var complete = reader.diffQueryAgainstStore({
                store: store,
                query: unionQuery,
                returnPartialData: false,
                fragmentMatcherFunction: fragmentMatcherFunction,
            }).complete;
            expect(complete).toBe(false);
        }, /IntrospectionFragmentMatcher/);
    });
    it('does not error on a query with fields missing from all but one named fragment', function () {
        var firstQuery = graphql_tag_1.default(templateObject_10 || (templateObject_10 = tslib_1.__makeTemplateObject(["\n      query {\n        person {\n          __typename\n          firstName\n          lastName\n        }\n      }\n    "], ["\n      query {\n        person {\n          __typename\n          firstName\n          lastName\n        }\n      }\n    "])));
        var firstResult = {
            person: {
                __typename: 'Author',
                firstName: 'John',
                lastName: 'Smith',
            },
        };
        var store = writer.writeQueryToStore({
            result: firstResult,
            query: firstQuery,
        });
        var unionQuery = graphql_tag_1.default(templateObject_11 || (templateObject_11 = tslib_1.__makeTemplateObject(["\n      query {\n        person {\n          __typename\n          ...authorInfo\n          ...jediInfo\n        }\n      }\n\n      fragment authorInfo on Author {\n        firstName\n      }\n\n      fragment jediInfo on Jedi {\n        powers\n      }\n    "], ["\n      query {\n        person {\n          __typename\n          ...authorInfo\n          ...jediInfo\n        }\n      }\n\n      fragment authorInfo on Author {\n        firstName\n      }\n\n      fragment jediInfo on Jedi {\n        powers\n      }\n    "])));
        var complete = reader.diffQueryAgainstStore({
            store: store,
            query: unionQuery,
        }).complete;
        expect(complete).toBe(false);
    });
    it('throws an error on a query with fields missing from matching named fragments', function () {
        var firstQuery = graphql_tag_1.default(templateObject_12 || (templateObject_12 = tslib_1.__makeTemplateObject(["\n      query {\n        person {\n          __typename\n          firstName\n          lastName\n        }\n      }\n    "], ["\n      query {\n        person {\n          __typename\n          firstName\n          lastName\n        }\n      }\n    "])));
        var firstResult = {
            person: {
                __typename: 'Author',
                firstName: 'John',
                lastName: 'Smith',
            },
        };
        var store = writer.writeQueryToStore({
            result: firstResult,
            query: firstQuery,
        });
        var unionQuery = graphql_tag_1.default(templateObject_13 || (templateObject_13 = tslib_1.__makeTemplateObject(["\n      query {\n        person {\n          __typename\n          ...authorInfo2\n          ...jediInfo2\n        }\n      }\n\n      fragment authorInfo2 on Author {\n        firstName\n        address\n      }\n\n      fragment jediInfo2 on Jedi {\n        jedi\n      }\n    "], ["\n      query {\n        person {\n          __typename\n          ...authorInfo2\n          ...jediInfo2\n        }\n      }\n\n      fragment authorInfo2 on Author {\n        firstName\n        address\n      }\n\n      fragment jediInfo2 on Jedi {\n        jedi\n      }\n    "])));
        expect(function () {
            reader.diffQueryAgainstStore({
                store: store,
                query: unionQuery,
                returnPartialData: false,
            });
        }).toThrow();
    });
    it('returns available fields if returnPartialData is true', function () {
        var firstQuery = graphql_tag_1.default(templateObject_14 || (templateObject_14 = tslib_1.__makeTemplateObject(["\n      {\n        people_one(id: \"1\") {\n          __typename\n          id\n          name\n        }\n      }\n    "], ["\n      {\n        people_one(id: \"1\") {\n          __typename\n          id\n          name\n        }\n      }\n    "])));
        var firstResult = {
            people_one: {
                __typename: 'Person',
                id: 'lukeId',
                name: 'Luke Skywalker',
            },
        };
        var store = writer.writeQueryToStore({
            result: firstResult,
            query: firstQuery,
        });
        var simpleQuery = graphql_tag_1.default(templateObject_15 || (templateObject_15 = tslib_1.__makeTemplateObject(["\n      {\n        people_one(id: \"1\") {\n          name\n          age\n        }\n      }\n    "], ["\n      {\n        people_one(id: \"1\") {\n          name\n          age\n        }\n      }\n    "])));
        var inlineFragmentQuery = graphql_tag_1.default(templateObject_16 || (templateObject_16 = tslib_1.__makeTemplateObject(["\n      {\n        people_one(id: \"1\") {\n          ... on Person {\n            name\n            age\n          }\n        }\n      }\n    "], ["\n      {\n        people_one(id: \"1\") {\n          ... on Person {\n            name\n            age\n          }\n        }\n      }\n    "])));
        var namedFragmentQuery = graphql_tag_1.default(templateObject_17 || (templateObject_17 = tslib_1.__makeTemplateObject(["\n      query {\n        people_one(id: \"1\") {\n          ...personInfo\n        }\n      }\n\n      fragment personInfo on Person {\n        name\n        age\n      }\n    "], ["\n      query {\n        people_one(id: \"1\") {\n          ...personInfo\n        }\n      }\n\n      fragment personInfo on Person {\n        name\n        age\n      }\n    "])));
        var simpleDiff = reader.diffQueryAgainstStore({
            store: store,
            query: simpleQuery,
        });
        expect(simpleDiff.result).toEqual({
            people_one: {
                name: 'Luke Skywalker',
            },
        });
        var inlineDiff = reader.diffQueryAgainstStore({
            store: store,
            query: inlineFragmentQuery,
        });
        expect(inlineDiff.result).toEqual({
            people_one: {
                name: 'Luke Skywalker',
            },
        });
        var namedDiff = reader.diffQueryAgainstStore({
            store: store,
            query: namedFragmentQuery,
        });
        expect(namedDiff.result).toEqual({
            people_one: {
                name: 'Luke Skywalker',
            },
        });
        expect(function () {
            reader.diffQueryAgainstStore({
                store: store,
                query: simpleQuery,
                returnPartialData: false,
            });
        }).toThrow();
    });
    it('will add a private id property', function () {
        var query = graphql_tag_1.default(templateObject_18 || (templateObject_18 = tslib_1.__makeTemplateObject(["\n      query {\n        a {\n          id\n          b\n        }\n        c {\n          d\n          e {\n            id\n            f\n          }\n          g {\n            h\n          }\n        }\n      }\n    "], ["\n      query {\n        a {\n          id\n          b\n        }\n        c {\n          d\n          e {\n            id\n            f\n          }\n          g {\n            h\n          }\n        }\n      }\n    "])));
        var queryResult = {
            a: [{ id: 'a:1', b: 1.1 }, { id: 'a:2', b: 1.2 }, { id: 'a:3', b: 1.3 }],
            c: {
                d: 2,
                e: [
                    { id: 'e:1', f: 3.1 },
                    { id: 'e:2', f: 3.2 },
                    { id: 'e:3', f: 3.3 },
                    { id: 'e:4', f: 3.4 },
                    { id: 'e:5', f: 3.5 },
                ],
                g: { h: 4 },
            },
        };
        function dataIdFromObject(_a) {
            var id = _a.id;
            return id;
        }
        var store = writer.writeQueryToStore({
            query: query,
            result: queryResult,
            dataIdFromObject: dataIdFromObject,
        });
        var result = reader.diffQueryAgainstStore({
            store: store,
            query: query,
        }).result;
        expect(result).toEqual(queryResult);
        expect(dataIdFromObject(result.a[0])).toBe('a:1');
        expect(dataIdFromObject(result.a[1])).toBe('a:2');
        expect(dataIdFromObject(result.a[2])).toBe('a:3');
        expect(dataIdFromObject(result.c.e[0])).toBe('e:1');
        expect(dataIdFromObject(result.c.e[1])).toBe('e:2');
        expect(dataIdFromObject(result.c.e[2])).toBe('e:3');
        expect(dataIdFromObject(result.c.e[3])).toBe('e:4');
        expect(dataIdFromObject(result.c.e[4])).toBe('e:5');
    });
    describe('referential equality preservation', function () {
        it('will return the previous result if there are no changes', function () {
            var query = graphql_tag_1.default(templateObject_19 || (templateObject_19 = tslib_1.__makeTemplateObject(["\n        query {\n          a {\n            b\n          }\n          c {\n            d\n            e {\n              f\n            }\n          }\n        }\n      "], ["\n        query {\n          a {\n            b\n          }\n          c {\n            d\n            e {\n              f\n            }\n          }\n        }\n      "])));
            var queryResult = {
                a: { b: 1 },
                c: { d: 2, e: { f: 3 } },
            };
            var store = writer.writeQueryToStore({
                query: query,
                result: queryResult,
            });
            var previousResult = {
                a: { b: 1 },
                c: { d: 2, e: { f: 3 } },
            };
            var result = reader.diffQueryAgainstStore({
                store: store,
                query: query,
                previousResult: previousResult,
            }).result;
            expect(result).toEqual(queryResult);
            expect(result).toEqual(previousResult);
        });
        it('will return parts of the previous result that changed', function () {
            var query = graphql_tag_1.default(templateObject_20 || (templateObject_20 = tslib_1.__makeTemplateObject(["\n        query {\n          a {\n            b\n          }\n          c {\n            d\n            e {\n              f\n            }\n          }\n        }\n      "], ["\n        query {\n          a {\n            b\n          }\n          c {\n            d\n            e {\n              f\n            }\n          }\n        }\n      "])));
            var queryResult = {
                a: { b: 1 },
                c: { d: 2, e: { f: 3 } },
            };
            var store = writer.writeQueryToStore({
                query: query,
                result: queryResult,
            });
            var previousResult = {
                a: { b: 1 },
                c: { d: 20, e: { f: 3 } },
            };
            var result = reader.diffQueryAgainstStore({
                store: store,
                query: query,
                previousResult: previousResult,
            }).result;
            expect(result).toEqual(queryResult);
            expect(result).not.toEqual(previousResult);
            expect(result.a).toEqual(previousResult.a);
            expect(result.c).not.toEqual(previousResult.c);
            expect(result.c.e).toEqual(previousResult.c.e);
        });
        it('will return the previous result if there are no changes in child arrays', function () {
            var query = graphql_tag_1.default(templateObject_21 || (templateObject_21 = tslib_1.__makeTemplateObject(["\n        query {\n          a {\n            b\n          }\n          c {\n            d\n            e {\n              f\n            }\n          }\n        }\n      "], ["\n        query {\n          a {\n            b\n          }\n          c {\n            d\n            e {\n              f\n            }\n          }\n        }\n      "])));
            var queryResult = {
                a: [{ b: 1.1 }, { b: 1.2 }, { b: 1.3 }],
                c: {
                    d: 2,
                    e: [{ f: 3.1 }, { f: 3.2 }, { f: 3.3 }, { f: 3.4 }, { f: 3.5 }],
                },
            };
            var store = writer.writeQueryToStore({
                query: query,
                result: queryResult,
            });
            var previousResult = {
                a: [{ b: 1.1 }, { b: 1.2 }, { b: 1.3 }],
                c: {
                    d: 2,
                    e: [{ f: 3.1 }, { f: 3.2 }, { f: 3.3 }, { f: 3.4 }, { f: 3.5 }],
                },
            };
            var result = reader.diffQueryAgainstStore({
                store: store,
                query: query,
                previousResult: previousResult,
            }).result;
            expect(result).toEqual(queryResult);
            expect(result).toEqual(previousResult);
        });
        it('will not add zombie items when previousResult starts with the same items', function () {
            var query = graphql_tag_1.default(templateObject_22 || (templateObject_22 = tslib_1.__makeTemplateObject(["\n        query {\n          a {\n            b\n          }\n        }\n      "], ["\n        query {\n          a {\n            b\n          }\n        }\n      "])));
            var queryResult = {
                a: [{ b: 1.1 }, { b: 1.2 }],
            };
            var store = writer.writeQueryToStore({
                query: query,
                result: queryResult,
            });
            var previousResult = {
                a: [{ b: 1.1 }, { b: 1.2 }, { b: 1.3 }],
            };
            var result = reader.diffQueryAgainstStore({
                store: store,
                query: query,
                previousResult: previousResult,
            }).result;
            expect(result).toEqual(queryResult);
            expect(result.a[0]).toEqual(previousResult.a[0]);
            expect(result.a[1]).toEqual(previousResult.a[1]);
        });
        it('will return the previous result if there are no changes in nested child arrays', function () {
            var query = graphql_tag_1.default(templateObject_23 || (templateObject_23 = tslib_1.__makeTemplateObject(["\n        query {\n          a {\n            b\n          }\n          c {\n            d\n            e {\n              f\n            }\n          }\n        }\n      "], ["\n        query {\n          a {\n            b\n          }\n          c {\n            d\n            e {\n              f\n            }\n          }\n        }\n      "])));
            var queryResult = {
                a: [[[[[{ b: 1.1 }, { b: 1.2 }, { b: 1.3 }]]]]],
                c: {
                    d: 2,
                    e: [[{ f: 3.1 }, { f: 3.2 }, { f: 3.3 }], [{ f: 3.4 }, { f: 3.5 }]],
                },
            };
            var store = writer.writeQueryToStore({
                query: query,
                result: queryResult,
            });
            var previousResult = {
                a: [[[[[{ b: 1.1 }, { b: 1.2 }, { b: 1.3 }]]]]],
                c: {
                    d: 2,
                    e: [[{ f: 3.1 }, { f: 3.2 }, { f: 3.3 }], [{ f: 3.4 }, { f: 3.5 }]],
                },
            };
            var result = reader.diffQueryAgainstStore({
                store: store,
                query: query,
                previousResult: previousResult,
            }).result;
            expect(result).toEqual(queryResult);
            expect(result).toEqual(previousResult);
        });
        it('will return parts of the previous result if there are changes in child arrays', function () {
            var query = graphql_tag_1.default(templateObject_24 || (templateObject_24 = tslib_1.__makeTemplateObject(["\n        query {\n          a {\n            b\n          }\n          c {\n            d\n            e {\n              f\n            }\n          }\n        }\n      "], ["\n        query {\n          a {\n            b\n          }\n          c {\n            d\n            e {\n              f\n            }\n          }\n        }\n      "])));
            var queryResult = {
                a: [{ b: 1.1 }, { b: 1.2 }, { b: 1.3 }],
                c: {
                    d: 2,
                    e: [{ f: 3.1 }, { f: 3.2 }, { f: 3.3 }, { f: 3.4 }, { f: 3.5 }],
                },
            };
            var store = writer.writeQueryToStore({
                query: query,
                result: queryResult,
            });
            var previousResult = {
                a: [{ b: 1.1 }, { b: -1.2 }, { b: 1.3 }],
                c: {
                    d: 20,
                    e: [{ f: 3.1 }, { f: 3.2 }, { f: 3.3 }, { f: 3.4 }, { f: 3.5 }],
                },
            };
            var result = reader.diffQueryAgainstStore({
                store: store,
                query: query,
                previousResult: previousResult,
            }).result;
            expect(result).toEqual(queryResult);
            expect(result).not.toEqual(previousResult);
            expect(result.a).not.toEqual(previousResult.a);
            expect(result.a[0]).toEqual(previousResult.a[0]);
            expect(result.a[1]).not.toEqual(previousResult.a[1]);
            expect(result.a[2]).toEqual(previousResult.a[2]);
            expect(result.c).not.toEqual(previousResult.c);
            expect(result.c.e).toEqual(previousResult.c.e);
            expect(result.c.e[0]).toEqual(previousResult.c.e[0]);
            expect(result.c.e[1]).toEqual(previousResult.c.e[1]);
            expect(result.c.e[2]).toEqual(previousResult.c.e[2]);
            expect(result.c.e[3]).toEqual(previousResult.c.e[3]);
            expect(result.c.e[4]).toEqual(previousResult.c.e[4]);
        });
        it('will return the same items in a different order with `dataIdFromObject`', function () {
            var query = graphql_tag_1.default(templateObject_25 || (templateObject_25 = tslib_1.__makeTemplateObject(["\n        query {\n          a {\n            id\n            b\n          }\n          c {\n            d\n            e {\n              id\n              f\n            }\n            g {\n              h\n            }\n          }\n        }\n      "], ["\n        query {\n          a {\n            id\n            b\n          }\n          c {\n            d\n            e {\n              id\n              f\n            }\n            g {\n              h\n            }\n          }\n        }\n      "])));
            var queryResult = {
                a: [
                    { id: 'a:1', b: 1.1 },
                    { id: 'a:2', b: 1.2 },
                    { id: 'a:3', b: 1.3 },
                ],
                c: {
                    d: 2,
                    e: [
                        { id: 'e:1', f: 3.1 },
                        { id: 'e:2', f: 3.2 },
                        { id: 'e:3', f: 3.3 },
                        { id: 'e:4', f: 3.4 },
                        { id: 'e:5', f: 3.5 },
                    ],
                    g: { h: 4 },
                },
            };
            var store = writer.writeQueryToStore({
                query: query,
                result: queryResult,
                dataIdFromObject: function (_a) {
                    var id = _a.id;
                    return id;
                },
            });
            var previousResult = {
                a: [
                    { id: 'a:3', b: 1.3 },
                    { id: 'a:2', b: 1.2 },
                    { id: 'a:1', b: 1.1 },
                ],
                c: {
                    d: 2,
                    e: [
                        { id: 'e:4', f: 3.4 },
                        { id: 'e:2', f: 3.2 },
                        { id: 'e:5', f: 3.5 },
                        { id: 'e:3', f: 3.3 },
                        { id: 'e:1', f: 3.1 },
                    ],
                    g: { h: 4 },
                },
            };
            var result = reader.diffQueryAgainstStore({
                store: store,
                query: query,
                previousResult: previousResult,
            }).result;
            expect(result).toEqual(queryResult);
            expect(result).not.toEqual(previousResult);
            expect(result.a).not.toEqual(previousResult.a);
            expect(result.a[0]).toEqual(previousResult.a[2]);
            expect(result.a[1]).toEqual(previousResult.a[1]);
            expect(result.a[2]).toEqual(previousResult.a[0]);
            expect(result.c).not.toEqual(previousResult.c);
            expect(result.c.e).not.toEqual(previousResult.c.e);
            expect(result.c.e[0]).toEqual(previousResult.c.e[4]);
            expect(result.c.e[1]).toEqual(previousResult.c.e[1]);
            expect(result.c.e[2]).toEqual(previousResult.c.e[3]);
            expect(result.c.e[3]).toEqual(previousResult.c.e[0]);
            expect(result.c.e[4]).toEqual(previousResult.c.e[2]);
            expect(result.c.g).toEqual(previousResult.c.g);
        });
        it('will return the same JSON scalar field object', function () {
            var query = graphql_tag_1.default(templateObject_26 || (templateObject_26 = tslib_1.__makeTemplateObject(["\n        {\n          a {\n            b\n            c\n          }\n          d {\n            e\n            f\n          }\n        }\n      "], ["\n        {\n          a {\n            b\n            c\n          }\n          d {\n            e\n            f\n          }\n        }\n      "])));
            var queryResult = {
                a: { b: 1, c: { x: 2, y: 3, z: 4 } },
                d: { e: 5, f: { x: 6, y: 7, z: 8 } },
            };
            var store = writer.writeQueryToStore({
                query: query,
                result: queryResult,
            });
            var previousResult = {
                a: { b: 1, c: { x: 2, y: 3, z: 4 } },
                d: { e: 50, f: { x: 6, y: 7, z: 8 } },
            };
            var result = reader.diffQueryAgainstStore({
                store: store,
                query: query,
                previousResult: previousResult,
            }).result;
            expect(result).toEqual(queryResult);
            expect(result).not.toEqual(previousResult);
            expect(result.a).toEqual(previousResult.a);
            expect(result.d).not.toEqual(previousResult.d);
            expect(result.d.f).toEqual(previousResult.d.f);
        });
        it('will preserve equality with custom resolvers', function () {
            var listQuery = graphql_tag_1.default(templateObject_27 || (templateObject_27 = tslib_1.__makeTemplateObject(["\n        {\n          people {\n            id\n            name\n            __typename\n          }\n        }\n      "], ["\n        {\n          people {\n            id\n            name\n            __typename\n          }\n        }\n      "])));
            var listResult = {
                people: [
                    {
                        id: '4',
                        name: 'Luke Skywalker',
                        __typename: 'Person',
                    },
                ],
            };
            var itemQuery = graphql_tag_1.default(templateObject_28 || (templateObject_28 = tslib_1.__makeTemplateObject(["\n        {\n          person(id: 4) {\n            id\n            name\n            __typename\n          }\n        }\n      "], ["\n        {\n          person(id: 4) {\n            id\n            name\n            __typename\n          }\n        }\n      "])));
            var dataIdFromObject = function (obj) { return obj.id; };
            var store = writer.writeQueryToStore({
                query: listQuery,
                result: listResult,
                dataIdFromObject: dataIdFromObject,
            });
            var previousResult = {
                person: listResult.people[0],
            };
            var cacheRedirects = {
                Query: {
                    person: function (_, args) {
                        return apollo_utilities_1.toIdValue({ id: args['id'], typename: 'Person' });
                    },
                },
            };
            var config = { dataIdFromObject: dataIdFromObject, cacheRedirects: cacheRedirects };
            var result = reader.diffQueryAgainstStore({
                store: store,
                query: itemQuery,
                previousResult: previousResult,
                config: config,
            }).result;
            expect(result).toEqual(previousResult);
        });
    });
    describe('malformed queries', function () {
        it('throws for non-scalar query fields without selection sets', function () {
            var validQuery = graphql_tag_1.default(templateObject_29 || (templateObject_29 = tslib_1.__makeTemplateObject(["\n        query getMessageList {\n          messageList {\n            id\n            __typename\n            message\n          }\n        }\n      "], ["\n        query getMessageList {\n          messageList {\n            id\n            __typename\n            message\n          }\n        }\n      "])));
            var invalidQuery = graphql_tag_1.default(templateObject_30 || (templateObject_30 = tslib_1.__makeTemplateObject(["\n        query getMessageList {\n          # This field needs a selection set because its value is an array\n          # of non-scalar objects.\n          messageList\n        }\n      "], ["\n        query getMessageList {\n          # This field needs a selection set because its value is an array\n          # of non-scalar objects.\n          messageList\n        }\n      "])));
            var store = writer.writeQueryToStore({
                query: validQuery,
                result: {
                    messageList: [
                        {
                            id: 1,
                            __typename: 'Message',
                            message: 'hi',
                        },
                        {
                            id: 2,
                            __typename: 'Message',
                            message: 'hello',
                        },
                        {
                            id: 3,
                            __typename: 'Message',
                            message: 'hey',
                        },
                    ],
                },
            });
            try {
                reader.diffQueryAgainstStore({
                    store: store,
                    query: invalidQuery,
                });
                throw new Error('should have thrown');
            }
            catch (e) {
                expect(e.message).toEqual('Missing selection set for object of type Message returned for query field messageList');
            }
        });
    });
    describe('issue #4081', function () {
        it('should not return results containing cycles', function () {
            var company = {
                __typename: 'Company',
                id: 1,
                name: 'Apollo',
                users: [],
            };
            company.users.push({
                __typename: 'User',
                id: 1,
                name: 'Ben',
                company: company,
            }, {
                __typename: 'User',
                id: 2,
                name: 'James',
                company: company,
            });
            var query = graphql_tag_1.default(templateObject_31 || (templateObject_31 = tslib_1.__makeTemplateObject(["\n        query Query {\n          user {\n            ...UserFragment\n            company {\n              users {\n                ...UserFragment\n              }\n            }\n          }\n        }\n\n        fragment UserFragment on User {\n          id\n          name\n          company {\n            id\n            name\n          }\n        }\n      "], ["\n        query Query {\n          user {\n            ...UserFragment\n            company {\n              users {\n                ...UserFragment\n              }\n            }\n          }\n        }\n\n        fragment UserFragment on User {\n          id\n          name\n          company {\n            id\n            name\n          }\n        }\n      "])));
            function check(store) {
                var result = reader.diffQueryAgainstStore({ store: store, query: query }).result;
                var json = JSON.stringify(result);
                company.users.forEach(function (user) {
                    expect(json).toContain(JSON.stringify(user.name));
                });
                expect(result).toEqual({
                    user: {
                        id: 1,
                        name: 'Ben',
                        company: {
                            id: 1,
                            name: 'Apollo',
                            users: [
                                {
                                    id: 1,
                                    name: 'Ben',
                                    company: {
                                        id: 1,
                                        name: 'Apollo',
                                    },
                                },
                                {
                                    id: 2,
                                    name: 'James',
                                    company: {
                                        id: 1,
                                        name: 'Apollo',
                                    },
                                },
                            ],
                        },
                    },
                });
            }
            check(writer.writeQueryToStore({
                query: query,
                result: {
                    user: company.users[0],
                },
            }));
            check(writer.writeQueryToStore({
                dataIdFromObject: inMemoryCache_1.defaultDataIdFromObject,
                query: query,
                result: {
                    user: company.users[0],
                },
            }));
        });
    });
});
var templateObject_1, templateObject_2, templateObject_3, templateObject_4, templateObject_5, templateObject_6, templateObject_7, templateObject_8, templateObject_9, templateObject_10, templateObject_11, templateObject_12, templateObject_13, templateObject_14, templateObject_15, templateObject_16, templateObject_17, templateObject_18, templateObject_19, templateObject_20, templateObject_21, templateObject_22, templateObject_23, templateObject_24, templateObject_25, templateObject_26, templateObject_27, templateObject_28, templateObject_29, templateObject_30, templateObject_31;
//# sourceMappingURL=diffAgainstStore.js.map