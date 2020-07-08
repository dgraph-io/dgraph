"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var lodash_1 = require("lodash");
var graphql_tag_1 = tslib_1.__importDefault(require("graphql-tag"));
var apollo_utilities_1 = require("apollo-utilities");
var __1 = require("../");
var readFromStore_1 = require("../readFromStore");
var objectCache_1 = require("../objectCache");
var fragmentMatcherFunction = new __1.HeuristicFragmentMatcher().match;
var diffAgainstStore_1 = require("./diffAgainstStore");
describe('reading from the store', function () {
    var reader = new readFromStore_1.StoreReader();
    it('runs a nested query with proper fragment fields in arrays', function () {
        diffAgainstStore_1.withError(function () {
            var store = objectCache_1.defaultNormalizedCacheFactory({
                ROOT_QUERY: {
                    __typename: 'Query',
                    nestedObj: { type: 'id', id: 'abcde', generated: false },
                },
                abcde: {
                    id: 'abcde',
                    innerArray: [
                        { type: 'id', generated: true, id: 'abcde.innerArray.0' },
                    ],
                },
                'abcde.innerArray.0': {
                    id: 'abcdef',
                    someField: 3,
                },
            });
            var queryResult = reader.readQueryFromStore({
                store: store,
                query: graphql_tag_1.default(templateObject_1 || (templateObject_1 = tslib_1.__makeTemplateObject(["\n          {\n            ... on DummyQuery {\n              nestedObj {\n                innerArray {\n                  id\n                  otherField\n                }\n              }\n            }\n            ... on Query {\n              nestedObj {\n                innerArray {\n                  id\n                  someField\n                }\n              }\n            }\n            ... on DummyQuery2 {\n              nestedObj {\n                innerArray {\n                  id\n                  otherField2\n                }\n              }\n            }\n          }\n        "], ["\n          {\n            ... on DummyQuery {\n              nestedObj {\n                innerArray {\n                  id\n                  otherField\n                }\n              }\n            }\n            ... on Query {\n              nestedObj {\n                innerArray {\n                  id\n                  someField\n                }\n              }\n            }\n            ... on DummyQuery2 {\n              nestedObj {\n                innerArray {\n                  id\n                  otherField2\n                }\n              }\n            }\n          }\n        "]))),
                fragmentMatcherFunction: fragmentMatcherFunction,
            });
            expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
                nestedObj: {
                    innerArray: [{ id: 'abcdef', someField: 3 }],
                },
            });
        }, /queries contain union or interface types/);
    });
    it('rejects malformed queries', function () {
        expect(function () {
            reader.readQueryFromStore({
                store: objectCache_1.defaultNormalizedCacheFactory(),
                query: graphql_tag_1.default(templateObject_2 || (templateObject_2 = tslib_1.__makeTemplateObject(["\n          query {\n            name\n          }\n\n          query {\n            address\n          }\n        "], ["\n          query {\n            name\n          }\n\n          query {\n            address\n          }\n        "]))),
            });
        }).toThrowError(/2 operations/);
        expect(function () {
            reader.readQueryFromStore({
                store: objectCache_1.defaultNormalizedCacheFactory(),
                query: graphql_tag_1.default(templateObject_3 || (templateObject_3 = tslib_1.__makeTemplateObject(["\n          fragment x on y {\n            name\n          }\n        "], ["\n          fragment x on y {\n            name\n          }\n        "]))),
            });
        }).toThrowError(/contain a query/);
    });
    it('runs a basic query', function () {
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: result,
        });
        var queryResult = reader.readQueryFromStore({
            store: store,
            query: graphql_tag_1.default(templateObject_4 || (templateObject_4 = tslib_1.__makeTemplateObject(["\n        query {\n          stringField\n          numberField\n        }\n      "], ["\n        query {\n          stringField\n          numberField\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
            stringField: result['stringField'],
            numberField: result['numberField'],
        });
    });
    it('runs a basic query with arguments', function () {
        var query = graphql_tag_1.default(templateObject_5 || (templateObject_5 = tslib_1.__makeTemplateObject(["\n      query {\n        id\n        stringField(arg: $stringArg)\n        numberField(intArg: $intArg, floatArg: $floatArg)\n        nullField\n      }\n    "], ["\n      query {\n        id\n        stringField(arg: $stringArg)\n        numberField(intArg: $intArg, floatArg: $floatArg)\n        nullField\n      }\n    "])));
        var variables = {
            intArg: 5,
            floatArg: 3.14,
            stringArg: 'This is a string!',
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: {
                id: 'abcd',
                nullField: null,
                'numberField({"floatArg":3.14,"intArg":5})': 5,
                'stringField({"arg":"This is a string!"})': 'Heyo',
            },
        });
        var result = reader.readQueryFromStore({
            store: store,
            query: query,
            variables: variables,
        });
        expect(apollo_utilities_1.stripSymbols(result)).toEqual({
            id: 'abcd',
            nullField: null,
            numberField: 5,
            stringField: 'Heyo',
        });
    });
    it('runs a basic query with custom directives', function () {
        var query = graphql_tag_1.default(templateObject_6 || (templateObject_6 = tslib_1.__makeTemplateObject(["\n      query {\n        id\n        firstName @include(if: true)\n        lastName @upperCase\n        birthDate @dateFormat(format: \"DD-MM-YYYY\")\n      }\n    "], ["\n      query {\n        id\n        firstName @include(if: true)\n        lastName @upperCase\n        birthDate @dateFormat(format: \"DD-MM-YYYY\")\n      }\n    "])));
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: {
                id: 'abcd',
                firstName: 'James',
                'lastName@upperCase': 'BOND',
                'birthDate@dateFormat({"format":"DD-MM-YYYY"})': '20-05-1940',
            },
        });
        var result = reader.readQueryFromStore({
            store: store,
            query: query,
        });
        expect(apollo_utilities_1.stripSymbols(result)).toEqual({
            id: 'abcd',
            firstName: 'James',
            lastName: 'BOND',
            birthDate: '20-05-1940',
        });
    });
    it('runs a basic query with default values for arguments', function () {
        var query = graphql_tag_1.default(templateObject_7 || (templateObject_7 = tslib_1.__makeTemplateObject(["\n      query someBigQuery(\n        $stringArg: String = \"This is a default string!\"\n        $intArg: Int = 0\n        $floatArg: Float\n      ) {\n        id\n        stringField(arg: $stringArg)\n        numberField(intArg: $intArg, floatArg: $floatArg)\n        nullField\n      }\n    "], ["\n      query someBigQuery(\n        $stringArg: String = \"This is a default string!\"\n        $intArg: Int = 0\n        $floatArg: Float\n      ) {\n        id\n        stringField(arg: $stringArg)\n        numberField(intArg: $intArg, floatArg: $floatArg)\n        nullField\n      }\n    "])));
        var variables = {
            floatArg: 3.14,
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: {
                id: 'abcd',
                nullField: null,
                'numberField({"floatArg":3.14,"intArg":0})': 5,
                'stringField({"arg":"This is a default string!"})': 'Heyo',
            },
        });
        var result = reader.readQueryFromStore({
            store: store,
            query: query,
            variables: variables,
        });
        expect(apollo_utilities_1.stripSymbols(result)).toEqual({
            id: 'abcd',
            nullField: null,
            numberField: 5,
            stringField: 'Heyo',
        });
    });
    it('runs a nested query', function () {
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedObj: {
                id: 'abcde',
                stringField: 'This is a string too!',
                numberField: 6,
                nullField: null,
            },
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedObj')), {
                nestedObj: {
                    type: 'id',
                    id: 'abcde',
                    generated: false,
                },
            }),
            abcde: result.nestedObj,
        });
        var queryResult = reader.readQueryFromStore({
            store: store,
            query: graphql_tag_1.default(templateObject_8 || (templateObject_8 = tslib_1.__makeTemplateObject(["\n        {\n          stringField\n          numberField\n          nestedObj {\n            stringField\n            numberField\n          }\n        }\n      "], ["\n        {\n          stringField\n          numberField\n          nestedObj {\n            stringField\n            numberField\n          }\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
            stringField: 'This is a string!',
            numberField: 5,
            nestedObj: {
                stringField: 'This is a string too!',
                numberField: 6,
            },
        });
    });
    it('runs a nested query with multiple fragments', function () {
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedObj: {
                id: 'abcde',
                stringField: 'This is a string too!',
                numberField: 6,
                nullField: null,
            },
            deepNestedObj: {
                stringField: 'This is a deep string',
                numberField: 7,
                nullField: null,
            },
            nullObject: null,
            __typename: 'Item',
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedObj', 'deepNestedObj')), {
                __typename: 'Query',
                nestedObj: {
                    type: 'id',
                    id: 'abcde',
                    generated: false,
                },
            }),
            abcde: lodash_1.assign({}, result.nestedObj, {
                deepNestedObj: {
                    type: 'id',
                    id: 'abcdef',
                    generated: false,
                },
            }),
            abcdef: result.deepNestedObj,
        });
        var queryResult = reader.readQueryFromStore({
            store: store,
            query: graphql_tag_1.default(templateObject_9 || (templateObject_9 = tslib_1.__makeTemplateObject(["\n        {\n          stringField\n          numberField\n          nullField\n          ... on Query {\n            nestedObj {\n              stringField\n              nullField\n              deepNestedObj {\n                stringField\n                nullField\n              }\n            }\n          }\n          ... on Query {\n            nestedObj {\n              numberField\n              nullField\n              deepNestedObj {\n                numberField\n                nullField\n              }\n            }\n          }\n          ... on Query {\n            nullObject\n          }\n        }\n      "], ["\n        {\n          stringField\n          numberField\n          nullField\n          ... on Query {\n            nestedObj {\n              stringField\n              nullField\n              deepNestedObj {\n                stringField\n                nullField\n              }\n            }\n          }\n          ... on Query {\n            nestedObj {\n              numberField\n              nullField\n              deepNestedObj {\n                numberField\n                nullField\n              }\n            }\n          }\n          ... on Query {\n            nullObject\n          }\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedObj: {
                stringField: 'This is a string too!',
                numberField: 6,
                nullField: null,
                deepNestedObj: {
                    stringField: 'This is a deep string',
                    numberField: 7,
                    nullField: null,
                },
            },
            nullObject: null,
        });
    });
    it('runs a nested query with an array without IDs', function () {
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedArray: [
                {
                    stringField: 'This is a string too!',
                    numberField: 6,
                    nullField: null,
                },
                {
                    stringField: 'This is a string also!',
                    numberField: 7,
                    nullField: null,
                },
            ],
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedArray')), {
                nestedArray: [
                    { type: 'id', generated: true, id: 'abcd.nestedArray.0' },
                    { type: 'id', generated: true, id: 'abcd.nestedArray.1' },
                ],
            }),
            'abcd.nestedArray.0': result.nestedArray[0],
            'abcd.nestedArray.1': result.nestedArray[1],
        });
        var queryResult = reader.readQueryFromStore({
            store: store,
            query: graphql_tag_1.default(templateObject_10 || (templateObject_10 = tslib_1.__makeTemplateObject(["\n        {\n          stringField\n          numberField\n          nestedArray {\n            stringField\n            numberField\n          }\n        }\n      "], ["\n        {\n          stringField\n          numberField\n          nestedArray {\n            stringField\n            numberField\n          }\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
            stringField: 'This is a string!',
            numberField: 5,
            nestedArray: [
                {
                    stringField: 'This is a string too!',
                    numberField: 6,
                },
                {
                    stringField: 'This is a string also!',
                    numberField: 7,
                },
            ],
        });
    });
    it('runs a nested query with an array without IDs and a null', function () {
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedArray: [
                null,
                {
                    stringField: 'This is a string also!',
                    numberField: 7,
                    nullField: null,
                },
            ],
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedArray')), {
                nestedArray: [
                    null,
                    { type: 'id', generated: true, id: 'abcd.nestedArray.1' },
                ],
            }),
            'abcd.nestedArray.1': result.nestedArray[1],
        });
        var queryResult = reader.readQueryFromStore({
            store: store,
            query: graphql_tag_1.default(templateObject_11 || (templateObject_11 = tslib_1.__makeTemplateObject(["\n        {\n          stringField\n          numberField\n          nestedArray {\n            stringField\n            numberField\n          }\n        }\n      "], ["\n        {\n          stringField\n          numberField\n          nestedArray {\n            stringField\n            numberField\n          }\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
            stringField: 'This is a string!',
            numberField: 5,
            nestedArray: [
                null,
                {
                    stringField: 'This is a string also!',
                    numberField: 7,
                },
            ],
        });
    });
    it('runs a nested query with an array with IDs and a null', function () {
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedArray: [
                null,
                {
                    id: 'abcde',
                    stringField: 'This is a string also!',
                    numberField: 7,
                    nullField: null,
                },
            ],
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedArray')), {
                nestedArray: [null, { type: 'id', generated: false, id: 'abcde' }],
            }),
            abcde: result.nestedArray[1],
        });
        var queryResult = reader.readQueryFromStore({
            store: store,
            query: graphql_tag_1.default(templateObject_12 || (templateObject_12 = tslib_1.__makeTemplateObject(["\n        {\n          stringField\n          numberField\n          nestedArray {\n            id\n            stringField\n            numberField\n          }\n        }\n      "], ["\n        {\n          stringField\n          numberField\n          nestedArray {\n            id\n            stringField\n            numberField\n          }\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
            stringField: 'This is a string!',
            numberField: 5,
            nestedArray: [
                null,
                {
                    id: 'abcde',
                    stringField: 'This is a string also!',
                    numberField: 7,
                },
            ],
        });
    });
    it('throws on a missing field', function () {
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({ ROOT_QUERY: result });
        expect(function () {
            reader.readQueryFromStore({
                store: store,
                query: graphql_tag_1.default(templateObject_13 || (templateObject_13 = tslib_1.__makeTemplateObject(["\n          {\n            stringField\n            missingField\n          }\n        "], ["\n          {\n            stringField\n            missingField\n          }\n        "]))),
            });
        }).toThrowError(/field missingField on object/);
    });
    it('runs a nested query where the reference is null', function () {
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedObj: null,
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedObj')), {
                nestedObj: null,
            }),
        });
        var queryResult = reader.readQueryFromStore({
            store: store,
            query: graphql_tag_1.default(templateObject_14 || (templateObject_14 = tslib_1.__makeTemplateObject(["\n        {\n          stringField\n          numberField\n          nestedObj {\n            stringField\n            numberField\n          }\n        }\n      "], ["\n        {\n          stringField\n          numberField\n          nestedObj {\n            stringField\n            numberField\n          }\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
            stringField: 'This is a string!',
            numberField: 5,
            nestedObj: null,
        });
    });
    it('runs an array of non-objects', function () {
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            simpleArray: ['one', 'two', 'three'],
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'simpleArray')), {
                simpleArray: {
                    type: 'json',
                    json: result.simpleArray,
                },
            }),
        });
        var queryResult = reader.readQueryFromStore({
            store: store,
            query: graphql_tag_1.default(templateObject_15 || (templateObject_15 = tslib_1.__makeTemplateObject(["\n        {\n          stringField\n          numberField\n          simpleArray\n        }\n      "], ["\n        {\n          stringField\n          numberField\n          simpleArray\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
            stringField: 'This is a string!',
            numberField: 5,
            simpleArray: ['one', 'two', 'three'],
        });
    });
    it('runs an array of non-objects with null', function () {
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            simpleArray: [null, 'two', 'three'],
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'simpleArray')), {
                simpleArray: {
                    type: 'json',
                    json: result.simpleArray,
                },
            }),
        });
        var queryResult = reader.readQueryFromStore({
            store: store,
            query: graphql_tag_1.default(templateObject_16 || (templateObject_16 = tslib_1.__makeTemplateObject(["\n        {\n          stringField\n          numberField\n          simpleArray\n        }\n      "], ["\n        {\n          stringField\n          numberField\n          simpleArray\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
            stringField: 'This is a string!',
            numberField: 5,
            simpleArray: [null, 'two', 'three'],
        });
    });
    it('will read from an arbitrary root id', function () {
        var data = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedObj: {
                id: 'abcde',
                stringField: 'This is a string too!',
                numberField: 6,
                nullField: null,
            },
            deepNestedObj: {
                stringField: 'This is a deep string',
                numberField: 7,
                nullField: null,
            },
            nullObject: null,
            __typename: 'Item',
        };
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(data, 'nestedObj', 'deepNestedObj')), {
                __typename: 'Query',
                nestedObj: {
                    type: 'id',
                    id: 'abcde',
                    generated: false,
                },
            }),
            abcde: lodash_1.assign({}, data.nestedObj, {
                deepNestedObj: {
                    type: 'id',
                    id: 'abcdef',
                    generated: false,
                },
            }),
            abcdef: data.deepNestedObj,
        });
        var queryResult1 = reader.readQueryFromStore({
            store: store,
            rootId: 'abcde',
            query: graphql_tag_1.default(templateObject_17 || (templateObject_17 = tslib_1.__makeTemplateObject(["\n        {\n          stringField\n          numberField\n          nullField\n          deepNestedObj {\n            stringField\n            numberField\n            nullField\n          }\n        }\n      "], ["\n        {\n          stringField\n          numberField\n          nullField\n          deepNestedObj {\n            stringField\n            numberField\n            nullField\n          }\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult1)).toEqual({
            stringField: 'This is a string too!',
            numberField: 6,
            nullField: null,
            deepNestedObj: {
                stringField: 'This is a deep string',
                numberField: 7,
                nullField: null,
            },
        });
        var queryResult2 = reader.readQueryFromStore({
            store: store,
            rootId: 'abcdef',
            query: graphql_tag_1.default(templateObject_18 || (templateObject_18 = tslib_1.__makeTemplateObject(["\n        {\n          stringField\n          numberField\n          nullField\n        }\n      "], ["\n        {\n          stringField\n          numberField\n          nullField\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult2)).toEqual({
            stringField: 'This is a deep string',
            numberField: 7,
            nullField: null,
        });
    });
    it('properly handles the connection directive', function () {
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: {
                abc: [
                    {
                        generated: true,
                        id: 'ROOT_QUERY.abc.0',
                        type: 'id',
                    },
                ],
            },
            'ROOT_QUERY.abc.0': {
                name: 'efgh',
            },
        });
        var queryResult = reader.readQueryFromStore({
            store: store,
            query: graphql_tag_1.default(templateObject_19 || (templateObject_19 = tslib_1.__makeTemplateObject(["\n        {\n          books(skip: 0, limit: 2) @connection(key: \"abc\") {\n            name\n          }\n        }\n      "], ["\n        {\n          books(skip: 0, limit: 2) @connection(key: \"abc\") {\n            name\n          }\n        }\n      "]))),
        });
        expect(apollo_utilities_1.stripSymbols(queryResult)).toEqual({
            books: [
                {
                    name: 'efgh',
                },
            ],
        });
    });
});
var templateObject_1, templateObject_2, templateObject_3, templateObject_4, templateObject_5, templateObject_6, templateObject_7, templateObject_8, templateObject_9, templateObject_10, templateObject_11, templateObject_12, templateObject_13, templateObject_14, templateObject_15, templateObject_16, templateObject_17, templateObject_18, templateObject_19;
//# sourceMappingURL=readFromStore.js.map