"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
var tslib_1 = require("tslib");
var lodash_1 = require("lodash");
var graphql_tag_1 = tslib_1.__importDefault(require("graphql-tag"));
var apollo_utilities_1 = require("apollo-utilities");
var writeToStore_1 = require("../writeToStore");
var objectCache_1 = require("../objectCache");
var __1 = require("../");
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
var getIdField = function (_a) {
    var id = _a.id;
    return id;
};
describe('writing to the store', function () {
    var writer = new writeToStore_1.StoreWriter();
    it('properly normalizes a trivial item', function () {
        var query = graphql_tag_1.default(templateObject_1 || (templateObject_1 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n      }\n    "])));
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
        };
        expect(writer
            .writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
        })
            .toObject()).toEqual({
            ROOT_QUERY: result,
        });
    });
    it('properly normalizes an aliased field', function () {
        var query = graphql_tag_1.default(templateObject_2 || (templateObject_2 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        aliasedField: stringField\n        numberField\n        nullField\n      }\n    "], ["\n      {\n        id\n        aliasedField: stringField\n        numberField\n        nullField\n      }\n    "])));
        var result = {
            id: 'abcd',
            aliasedField: 'This is a string!',
            numberField: 5,
            nullField: null,
        };
        var normalized = writer.writeQueryToStore({
            result: result,
            query: query,
        });
        expect(normalized.toObject()).toEqual({
            ROOT_QUERY: {
                id: 'abcd',
                stringField: 'This is a string!',
                numberField: 5,
                nullField: null,
            },
        });
    });
    it('properly normalizes a aliased fields with arguments', function () {
        var query = graphql_tag_1.default(templateObject_3 || (templateObject_3 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        aliasedField1: stringField(arg: 1)\n        aliasedField2: stringField(arg: 2)\n        numberField\n        nullField\n      }\n    "], ["\n      {\n        id\n        aliasedField1: stringField(arg: 1)\n        aliasedField2: stringField(arg: 2)\n        numberField\n        nullField\n      }\n    "])));
        var result = {
            id: 'abcd',
            aliasedField1: 'The arg was 1!',
            aliasedField2: 'The arg was 2!',
            numberField: 5,
            nullField: null,
        };
        var normalized = writer.writeQueryToStore({
            result: result,
            query: query,
        });
        expect(normalized.toObject()).toEqual({
            ROOT_QUERY: {
                id: 'abcd',
                'stringField({"arg":1})': 'The arg was 1!',
                'stringField({"arg":2})': 'The arg was 2!',
                numberField: 5,
                nullField: null,
            },
        });
    });
    it('properly normalizes a query with variables', function () {
        var query = graphql_tag_1.default(templateObject_4 || (templateObject_4 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField(arg: $stringArg)\n        numberField(intArg: $intArg, floatArg: $floatArg)\n        nullField\n      }\n    "], ["\n      {\n        id\n        stringField(arg: $stringArg)\n        numberField(intArg: $intArg, floatArg: $floatArg)\n        nullField\n      }\n    "])));
        var variables = {
            intArg: 5,
            floatArg: 3.14,
            stringArg: 'This is a string!',
        };
        var result = {
            id: 'abcd',
            stringField: 'Heyo',
            numberField: 5,
            nullField: null,
        };
        var normalized = writer.writeQueryToStore({
            result: result,
            query: query,
            variables: variables,
        });
        expect(normalized.toObject()).toEqual({
            ROOT_QUERY: {
                id: 'abcd',
                nullField: null,
                'numberField({"floatArg":3.14,"intArg":5})': 5,
                'stringField({"arg":"This is a string!"})': 'Heyo',
            },
        });
    });
    it('properly normalizes a query with default values', function () {
        var query = graphql_tag_1.default(templateObject_5 || (templateObject_5 = tslib_1.__makeTemplateObject(["\n      query someBigQuery(\n        $stringArg: String = \"This is a default string!\"\n        $intArg: Int\n        $floatArg: Float\n      ) {\n        id\n        stringField(arg: $stringArg)\n        numberField(intArg: $intArg, floatArg: $floatArg)\n        nullField\n      }\n    "], ["\n      query someBigQuery(\n        $stringArg: String = \"This is a default string!\"\n        $intArg: Int\n        $floatArg: Float\n      ) {\n        id\n        stringField(arg: $stringArg)\n        numberField(intArg: $intArg, floatArg: $floatArg)\n        nullField\n      }\n    "])));
        var variables = {
            intArg: 5,
            floatArg: 3.14,
        };
        var result = {
            id: 'abcd',
            stringField: 'Heyo',
            numberField: 5,
            nullField: null,
        };
        var normalized = writer.writeQueryToStore({
            result: result,
            query: query,
            variables: variables,
        });
        expect(normalized.toObject()).toEqual({
            ROOT_QUERY: {
                id: 'abcd',
                nullField: null,
                'numberField({"floatArg":3.14,"intArg":5})': 5,
                'stringField({"arg":"This is a default string!"})': 'Heyo',
            },
        });
    });
    it('properly normalizes a query with custom directives', function () {
        var query = graphql_tag_1.default(templateObject_6 || (templateObject_6 = tslib_1.__makeTemplateObject(["\n      query {\n        id\n        firstName @include(if: true)\n        lastName @upperCase\n        birthDate @dateFormat(format: \"DD-MM-YYYY\")\n      }\n    "], ["\n      query {\n        id\n        firstName @include(if: true)\n        lastName @upperCase\n        birthDate @dateFormat(format: \"DD-MM-YYYY\")\n      }\n    "])));
        var result = {
            id: 'abcd',
            firstName: 'James',
            lastName: 'BOND',
            birthDate: '20-05-1940',
        };
        var normalized = writer.writeQueryToStore({
            result: result,
            query: query,
        });
        expect(normalized.toObject()).toEqual({
            ROOT_QUERY: {
                id: 'abcd',
                firstName: 'James',
                'lastName@upperCase': 'BOND',
                'birthDate@dateFormat({"format":"DD-MM-YYYY"})': '20-05-1940',
            },
        });
    });
    it('properly normalizes a nested object with an ID', function () {
        var _a;
        var query = graphql_tag_1.default(templateObject_7 || (templateObject_7 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedObj {\n          id\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedObj {\n          id\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "])));
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
        expect(writer
            .writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
            dataIdFromObject: getIdField,
        })
            .toObject()).toEqual((_a = {
                ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedObj')), {
                    nestedObj: {
                        type: 'id',
                        id: result.nestedObj.id,
                        generated: false,
                    },
                })
            },
            _a[result.nestedObj.id] = result.nestedObj,
            _a));
    });
    it('properly normalizes a nested object without an ID', function () {
        var _a;
        var query = graphql_tag_1.default(templateObject_8 || (templateObject_8 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedObj {\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedObj {\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "])));
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedObj: {
                stringField: 'This is a string too!',
                numberField: 6,
                nullField: null,
            },
        };
        expect(writer
            .writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
        })
            .toObject()).toEqual((_a = {
                ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedObj')), {
                    nestedObj: {
                        type: 'id',
                        id: "$ROOT_QUERY.nestedObj",
                        generated: true,
                    },
                })
            },
            _a["$ROOT_QUERY.nestedObj"] = result.nestedObj,
            _a));
    });
    it('properly normalizes a nested object with arguments but without an ID', function () {
        var _a;
        var query = graphql_tag_1.default(templateObject_9 || (templateObject_9 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedObj(arg: \"val\") {\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedObj(arg: \"val\") {\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "])));
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedObj: {
                stringField: 'This is a string too!',
                numberField: 6,
                nullField: null,
            },
        };
        expect(writer
            .writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
        })
            .toObject()).toEqual((_a = {
                ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedObj')), {
                    'nestedObj({"arg":"val"})': {
                        type: 'id',
                        id: "$ROOT_QUERY.nestedObj({\"arg\":\"val\"})",
                        generated: true,
                    },
                })
            },
            _a["$ROOT_QUERY.nestedObj({\"arg\":\"val\"})"] = result.nestedObj,
            _a));
    });
    it('properly normalizes a nested array with IDs', function () {
        var _a;
        var query = graphql_tag_1.default(templateObject_10 || (templateObject_10 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedArray {\n          id\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedArray {\n          id\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "])));
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedArray: [
                {
                    id: 'abcde',
                    stringField: 'This is a string too!',
                    numberField: 6,
                    nullField: null,
                },
                {
                    id: 'abcdef',
                    stringField: 'This is a string also!',
                    numberField: 7,
                    nullField: null,
                },
            ],
        };
        expect(writer
            .writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
            dataIdFromObject: getIdField,
        })
            .toObject()).toEqual((_a = {
                ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedArray')), {
                    nestedArray: result.nestedArray.map(function (obj) { return ({
                        type: 'id',
                        id: obj.id,
                        generated: false,
                    }); }),
                })
            },
            _a[result.nestedArray[0].id] = result.nestedArray[0],
            _a[result.nestedArray[1].id] = result.nestedArray[1],
            _a));
    });
    it('properly normalizes a nested array with IDs and a null', function () {
        var _a;
        var query = graphql_tag_1.default(templateObject_11 || (templateObject_11 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedArray {\n          id\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedArray {\n          id\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "])));
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedArray: [
                {
                    id: 'abcde',
                    stringField: 'This is a string too!',
                    numberField: 6,
                    nullField: null,
                },
                null,
            ],
        };
        expect(writer
            .writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
            dataIdFromObject: getIdField,
        })
            .toObject()).toEqual((_a = {
                ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedArray')), {
                    nestedArray: [
                        { type: 'id', id: result.nestedArray[0].id, generated: false },
                        null,
                    ],
                })
            },
            _a[result.nestedArray[0].id] = result.nestedArray[0],
            _a));
    });
    it('properly normalizes a nested array without IDs', function () {
        var _a;
        var query = graphql_tag_1.default(templateObject_12 || (templateObject_12 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedArray {\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedArray {\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "])));
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
        var normalized = writer.writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
        });
        expect(normalized.toObject()).toEqual((_a = {
                ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedArray')), {
                    nestedArray: [
                        { type: 'id', generated: true, id: "ROOT_QUERY.nestedArray.0" },
                        { type: 'id', generated: true, id: "ROOT_QUERY.nestedArray.1" },
                    ],
                })
            },
            _a["ROOT_QUERY.nestedArray.0"] = result.nestedArray[0],
            _a["ROOT_QUERY.nestedArray.1"] = result.nestedArray[1],
            _a));
    });
    it('properly normalizes a nested array without IDs and a null item', function () {
        var _a;
        var query = graphql_tag_1.default(templateObject_13 || (templateObject_13 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedArray {\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedArray {\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "])));
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
        var normalized = writer.writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
        });
        expect(normalized.toObject()).toEqual((_a = {
                ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedArray')), {
                    nestedArray: [
                        null,
                        { type: 'id', generated: true, id: "ROOT_QUERY.nestedArray.1" },
                    ],
                })
            },
            _a["ROOT_QUERY.nestedArray.1"] = result.nestedArray[1],
            _a));
    });
    it('properly normalizes an array of non-objects', function () {
        var query = graphql_tag_1.default(templateObject_14 || (templateObject_14 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        simpleArray\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        simpleArray\n      }\n    "])));
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            simpleArray: ['one', 'two', 'three'],
        };
        var normalized = writer.writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
            dataIdFromObject: getIdField,
        });
        expect(normalized.toObject()).toEqual({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'simpleArray')), {
                simpleArray: {
                    type: 'json',
                    json: [
                        result.simpleArray[0],
                        result.simpleArray[1],
                        result.simpleArray[2],
                    ],
                },
            }),
        });
    });
    it('properly normalizes an array of non-objects with null', function () {
        var query = graphql_tag_1.default(templateObject_15 || (templateObject_15 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        simpleArray\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        simpleArray\n      }\n    "])));
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            simpleArray: [null, 'two', 'three'],
        };
        var normalized = writer.writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
        });
        expect(normalized.toObject()).toEqual({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'simpleArray')), {
                simpleArray: {
                    type: 'json',
                    json: [
                        result.simpleArray[0],
                        result.simpleArray[1],
                        result.simpleArray[2],
                    ],
                },
            }),
        });
    });
    it('properly normalizes an object occurring in different graphql paths twice', function () {
        var query = graphql_tag_1.default(templateObject_16 || (templateObject_16 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        object1 {\n          id\n          stringField\n        }\n        object2 {\n          id\n          numberField\n        }\n      }\n    "], ["\n      {\n        id\n        object1 {\n          id\n          stringField\n        }\n        object2 {\n          id\n          numberField\n        }\n      }\n    "])));
        var result = {
            id: 'a',
            object1: {
                id: 'aa',
                stringField: 'string',
            },
            object2: {
                id: 'aa',
                numberField: 1,
            },
        };
        var normalized = writer.writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
            dataIdFromObject: getIdField,
        });
        expect(normalized.toObject()).toEqual({
            ROOT_QUERY: {
                id: 'a',
                object1: {
                    type: 'id',
                    id: 'aa',
                    generated: false,
                },
                object2: {
                    type: 'id',
                    id: 'aa',
                    generated: false,
                },
            },
            aa: {
                id: 'aa',
                stringField: 'string',
                numberField: 1,
            },
        });
    });
    it('properly normalizes an object occurring in different graphql array paths twice', function () {
        var query = graphql_tag_1.default(templateObject_17 || (templateObject_17 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        array1 {\n          id\n          stringField\n          obj {\n            id\n            stringField\n          }\n        }\n        array2 {\n          id\n          stringField\n          obj {\n            id\n            numberField\n          }\n        }\n      }\n    "], ["\n      {\n        id\n        array1 {\n          id\n          stringField\n          obj {\n            id\n            stringField\n          }\n        }\n        array2 {\n          id\n          stringField\n          obj {\n            id\n            numberField\n          }\n        }\n      }\n    "])));
        var result = {
            id: 'a',
            array1: [
                {
                    id: 'aa',
                    stringField: 'string',
                    obj: {
                        id: 'aaa',
                        stringField: 'string',
                    },
                },
            ],
            array2: [
                {
                    id: 'ab',
                    stringField: 'string2',
                    obj: {
                        id: 'aaa',
                        numberField: 1,
                    },
                },
            ],
        };
        var normalized = writer.writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
            dataIdFromObject: getIdField,
        });
        expect(normalized.toObject()).toEqual({
            ROOT_QUERY: {
                id: 'a',
                array1: [
                    {
                        type: 'id',
                        id: 'aa',
                        generated: false,
                    },
                ],
                array2: [
                    {
                        type: 'id',
                        id: 'ab',
                        generated: false,
                    },
                ],
            },
            aa: {
                id: 'aa',
                stringField: 'string',
                obj: {
                    type: 'id',
                    id: 'aaa',
                    generated: false,
                },
            },
            ab: {
                id: 'ab',
                stringField: 'string2',
                obj: {
                    type: 'id',
                    id: 'aaa',
                    generated: false,
                },
            },
            aaa: {
                id: 'aaa',
                stringField: 'string',
                numberField: 1,
            },
        });
    });
    it('properly normalizes an object occurring in the same graphql array path twice', function () {
        var query = graphql_tag_1.default(templateObject_18 || (templateObject_18 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        array1 {\n          id\n          stringField\n          obj {\n            id\n            stringField\n            numberField\n          }\n        }\n      }\n    "], ["\n      {\n        id\n        array1 {\n          id\n          stringField\n          obj {\n            id\n            stringField\n            numberField\n          }\n        }\n      }\n    "])));
        var result = {
            id: 'a',
            array1: [
                {
                    id: 'aa',
                    stringField: 'string',
                    obj: {
                        id: 'aaa',
                        stringField: 'string',
                        numberField: 1,
                    },
                },
                {
                    id: 'ab',
                    stringField: 'string2',
                    obj: {
                        id: 'aaa',
                        stringField: 'should not be written',
                        numberField: 2,
                    },
                },
            ],
        };
        var normalized = writer.writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
            dataIdFromObject: getIdField,
        });
        expect(normalized.toObject()).toEqual({
            ROOT_QUERY: {
                id: 'a',
                array1: [
                    {
                        type: 'id',
                        id: 'aa',
                        generated: false,
                    },
                    {
                        type: 'id',
                        id: 'ab',
                        generated: false,
                    },
                ],
            },
            aa: {
                id: 'aa',
                stringField: 'string',
                obj: {
                    type: 'id',
                    id: 'aaa',
                    generated: false,
                },
            },
            ab: {
                id: 'ab',
                stringField: 'string2',
                obj: {
                    type: 'id',
                    id: 'aaa',
                    generated: false,
                },
            },
            aaa: {
                id: 'aaa',
                stringField: 'string',
                numberField: 1,
            },
        });
    });
    it('merges nodes', function () {
        var query = graphql_tag_1.default(templateObject_19 || (templateObject_19 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        numberField\n        nullField\n      }\n    "], ["\n      {\n        id\n        numberField\n        nullField\n      }\n    "])));
        var result = {
            id: 'abcd',
            numberField: 5,
            nullField: null,
        };
        var store = writer.writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
            dataIdFromObject: getIdField,
        });
        var query2 = graphql_tag_1.default(templateObject_20 || (templateObject_20 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        nullField\n      }\n    "], ["\n      {\n        id\n        stringField\n        nullField\n      }\n    "])));
        var result2 = {
            id: 'abcd',
            stringField: 'This is a string!',
            nullField: null,
        };
        var store2 = writer.writeQueryToStore({
            store: store,
            query: query2,
            result: result2,
            dataIdFromObject: getIdField,
        });
        expect(store2.toObject()).toEqual({
            ROOT_QUERY: lodash_1.assign({}, result, result2),
        });
    });
    it('properly normalizes a nested object that returns null', function () {
        var query = graphql_tag_1.default(templateObject_21 || (templateObject_21 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedObj {\n          id\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n        nestedObj {\n          id\n          stringField\n          numberField\n          nullField\n        }\n      }\n    "])));
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
            nestedObj: null,
        };
        expect(writer
            .writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
        })
            .toObject()).toEqual({
            ROOT_QUERY: lodash_1.assign({}, lodash_1.assign({}, lodash_1.omit(result, 'nestedObj')), {
                nestedObj: null,
            }),
        });
    });
    it('properly normalizes an object with an ID when no extension is passed', function () {
        var query = graphql_tag_1.default(templateObject_22 || (templateObject_22 = tslib_1.__makeTemplateObject(["\n      {\n        people_one(id: \"5\") {\n          id\n          stringField\n        }\n      }\n    "], ["\n      {\n        people_one(id: \"5\") {\n          id\n          stringField\n        }\n      }\n    "])));
        var result = {
            people_one: {
                id: 'abcd',
                stringField: 'This is a string!',
            },
        };
        expect(writer
            .writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
        })
            .toObject()).toEqual({
            ROOT_QUERY: {
                'people_one({"id":"5"})': {
                    type: 'id',
                    id: '$ROOT_QUERY.people_one({"id":"5"})',
                    generated: true,
                },
            },
            '$ROOT_QUERY.people_one({"id":"5"})': {
                id: 'abcd',
                stringField: 'This is a string!',
            },
        });
    });
    it('consistently serialize different types of input when passed inlined or as variable', function () {
        var testData = [
            {
                mutation: graphql_tag_1.default(templateObject_23 || (templateObject_23 = tslib_1.__makeTemplateObject(["\n          mutation mut($in: Int!) {\n            mut(inline: 5, variable: $in) {\n              id\n            }\n          }\n        "], ["\n          mutation mut($in: Int!) {\n            mut(inline: 5, variable: $in) {\n              id\n            }\n          }\n        "]))),
                variables: { in: 5 },
                expected: 'mut({"inline":5,"variable":5})',
            },
            {
                mutation: graphql_tag_1.default(templateObject_24 || (templateObject_24 = tslib_1.__makeTemplateObject(["\n          mutation mut($in: Float!) {\n            mut(inline: 5.5, variable: $in) {\n              id\n            }\n          }\n        "], ["\n          mutation mut($in: Float!) {\n            mut(inline: 5.5, variable: $in) {\n              id\n            }\n          }\n        "]))),
                variables: { in: 5.5 },
                expected: 'mut({"inline":5.5,"variable":5.5})',
            },
            {
                mutation: graphql_tag_1.default(templateObject_25 || (templateObject_25 = tslib_1.__makeTemplateObject(["\n          mutation mut($in: String!) {\n            mut(inline: \"abc\", variable: $in) {\n              id\n            }\n          }\n        "], ["\n          mutation mut($in: String!) {\n            mut(inline: \"abc\", variable: $in) {\n              id\n            }\n          }\n        "]))),
                variables: { in: 'abc' },
                expected: 'mut({"inline":"abc","variable":"abc"})',
            },
            {
                mutation: graphql_tag_1.default(templateObject_26 || (templateObject_26 = tslib_1.__makeTemplateObject(["\n          mutation mut($in: Array!) {\n            mut(inline: [1, 2], variable: $in) {\n              id\n            }\n          }\n        "], ["\n          mutation mut($in: Array!) {\n            mut(inline: [1, 2], variable: $in) {\n              id\n            }\n          }\n        "]))),
                variables: { in: [1, 2] },
                expected: 'mut({"inline":[1,2],"variable":[1,2]})',
            },
            {
                mutation: graphql_tag_1.default(templateObject_27 || (templateObject_27 = tslib_1.__makeTemplateObject(["\n          mutation mut($in: Object!) {\n            mut(inline: { a: 1 }, variable: $in) {\n              id\n            }\n          }\n        "], ["\n          mutation mut($in: Object!) {\n            mut(inline: { a: 1 }, variable: $in) {\n              id\n            }\n          }\n        "]))),
                variables: { in: { a: 1 } },
                expected: 'mut({"inline":{"a":1},"variable":{"a":1}})',
            },
            {
                mutation: graphql_tag_1.default(templateObject_28 || (templateObject_28 = tslib_1.__makeTemplateObject(["\n          mutation mut($in: Boolean!) {\n            mut(inline: true, variable: $in) {\n              id\n            }\n          }\n        "], ["\n          mutation mut($in: Boolean!) {\n            mut(inline: true, variable: $in) {\n              id\n            }\n          }\n        "]))),
                variables: { in: true },
                expected: 'mut({"inline":true,"variable":true})',
            },
        ];
        function isOperationDefinition(definition) {
            return definition.kind === 'OperationDefinition';
        }
        function isField(selection) {
            return selection.kind === 'Field';
        }
        testData.forEach(function (data) {
            data.mutation.definitions.forEach(function (definition) {
                if (isOperationDefinition(definition)) {
                    definition.selectionSet.selections.forEach(function (selection) {
                        if (isField(selection)) {
                            expect(apollo_utilities_1.storeKeyNameFromField(selection, data.variables)).toEqual(data.expected);
                        }
                    });
                }
            });
        });
    });
    it('properly normalizes a mutation with object or array parameters and variables', function () {
        var mutation = graphql_tag_1.default(templateObject_29 || (templateObject_29 = tslib_1.__makeTemplateObject(["\n      mutation some_mutation($nil: ID, $in: Object) {\n        some_mutation(\n          input: {\n            id: \"5\"\n            arr: [1, { a: \"b\" }]\n            obj: { a: \"b\" }\n            num: 5.5\n            nil: $nil\n            bo: true\n          }\n        ) {\n          id\n        }\n        some_mutation_with_variables(input: $in) {\n          id\n        }\n      }\n    "], ["\n      mutation some_mutation($nil: ID, $in: Object) {\n        some_mutation(\n          input: {\n            id: \"5\"\n            arr: [1, { a: \"b\" }]\n            obj: { a: \"b\" }\n            num: 5.5\n            nil: $nil\n            bo: true\n          }\n        ) {\n          id\n        }\n        some_mutation_with_variables(input: $in) {\n          id\n        }\n      }\n    "])));
        var result = {
            some_mutation: {
                id: 'id',
            },
            some_mutation_with_variables: {
                id: 'id',
            },
        };
        var variables = {
            nil: null,
            in: {
                id: '5',
                arr: [1, { a: 'b' }],
                obj: { a: 'b' },
                num: 5.5,
                nil: null,
                bo: true,
            },
        };
        function isOperationDefinition(value) {
            return value.kind === 'OperationDefinition';
        }
        mutation.definitions.map(function (def) {
            if (isOperationDefinition(def)) {
                expect(writer
                    .writeSelectionSetToStore({
                    dataId: '5',
                    selectionSet: def.selectionSet,
                    result: lodash_1.cloneDeep(result),
                    context: {
                        store: objectCache_1.defaultNormalizedCacheFactory(),
                        variables: variables,
                        dataIdFromObject: function () { return '5'; },
                    },
                })
                    .toObject()).toEqual({
                    '5': {
                        id: 'id',
                        'some_mutation({"input":{"arr":[1,{"a":"b"}],"bo":true,"id":"5","nil":null,"num":5.5,"obj":{"a":"b"}}})': {
                            generated: false,
                            id: '5',
                            type: 'id',
                        },
                        'some_mutation_with_variables({"input":{"arr":[1,{"a":"b"}],"bo":true,"id":"5","nil":null,"num":5.5,"obj":{"a":"b"}}})': {
                            generated: false,
                            id: '5',
                            type: 'id',
                        },
                    },
                });
            }
            else {
                throw 'No operation definition found';
            }
        });
    });
    it('should write to store if `dataIdFromObject` returns an ID of 0', function () {
        var query = graphql_tag_1.default(templateObject_30 || (templateObject_30 = tslib_1.__makeTemplateObject(["\n      query {\n        author {\n          firstName\n          id\n          __typename\n        }\n      }\n    "], ["\n      query {\n        author {\n          firstName\n          id\n          __typename\n        }\n      }\n    "])));
        var data = {
            author: {
                id: 0,
                __typename: 'Author',
                firstName: 'John',
            },
        };
        var expStore = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: {
                author: {
                    id: 0,
                    typename: 'Author',
                    type: 'id',
                    generated: false,
                },
            },
            0: {
                id: data.author.id,
                __typename: data.author.__typename,
                firstName: data.author.firstName,
            },
        });
        expect(writer
            .writeQueryToStore({
            result: data,
            query: query,
            dataIdFromObject: function () { return 0; },
        })
            .toObject()).toEqual(expStore.toObject());
    });
    describe('type escaping', function () {
        var dataIdFromObject = function (object) {
            if (object.__typename && object.id) {
                return object.__typename + '__' + object.id;
            }
            return undefined;
        };
        it('should correctly escape generated ids', function () {
            var query = graphql_tag_1.default(templateObject_31 || (templateObject_31 = tslib_1.__makeTemplateObject(["\n        query {\n          author {\n            firstName\n            lastName\n          }\n        }\n      "], ["\n        query {\n          author {\n            firstName\n            lastName\n          }\n        }\n      "])));
            var data = {
                author: {
                    firstName: 'John',
                    lastName: 'Smith',
                },
            };
            var expStore = objectCache_1.defaultNormalizedCacheFactory({
                ROOT_QUERY: {
                    author: {
                        type: 'id',
                        id: '$ROOT_QUERY.author',
                        generated: true,
                    },
                },
                '$ROOT_QUERY.author': data.author,
            });
            expect(writer
                .writeQueryToStore({
                result: data,
                query: query,
            })
                .toObject()).toEqual(expStore.toObject());
        });
        it('should correctly escape real ids', function () {
            var _a;
            var query = graphql_tag_1.default(templateObject_32 || (templateObject_32 = tslib_1.__makeTemplateObject(["\n        query {\n          author {\n            firstName\n            id\n            __typename\n          }\n        }\n      "], ["\n        query {\n          author {\n            firstName\n            id\n            __typename\n          }\n        }\n      "])));
            var data = {
                author: {
                    firstName: 'John',
                    id: '129',
                    __typename: 'Author',
                },
            };
            var expStore = objectCache_1.defaultNormalizedCacheFactory((_a = {
                    ROOT_QUERY: {
                        author: {
                            type: 'id',
                            id: dataIdFromObject(data.author),
                            generated: false,
                            typename: 'Author',
                        },
                    }
                },
                _a[dataIdFromObject(data.author)] = {
                    firstName: data.author.firstName,
                    id: data.author.id,
                    __typename: data.author.__typename,
                },
                _a));
            expect(writer
                .writeQueryToStore({
                result: data,
                query: query,
                dataIdFromObject: dataIdFromObject,
            })
                .toObject()).toEqual(expStore.toObject());
        });
        it('should correctly escape json blobs', function () {
            var _a;
            var query = graphql_tag_1.default(templateObject_33 || (templateObject_33 = tslib_1.__makeTemplateObject(["\n        query {\n          author {\n            info\n            id\n            __typename\n          }\n        }\n      "], ["\n        query {\n          author {\n            info\n            id\n            __typename\n          }\n        }\n      "])));
            var data = {
                author: {
                    info: {
                        name: 'John',
                    },
                    id: '129',
                    __typename: 'Author',
                },
            };
            var expStore = objectCache_1.defaultNormalizedCacheFactory((_a = {
                    ROOT_QUERY: {
                        author: {
                            type: 'id',
                            id: dataIdFromObject(data.author),
                            generated: false,
                            typename: 'Author',
                        },
                    }
                },
                _a[dataIdFromObject(data.author)] = {
                    __typename: data.author.__typename,
                    id: data.author.id,
                    info: {
                        type: 'json',
                        json: data.author.info,
                    },
                },
                _a));
            expect(writer
                .writeQueryToStore({
                result: data,
                query: query,
                dataIdFromObject: dataIdFromObject,
            })
                .toObject()).toEqual(expStore.toObject());
        });
    });
    it('should merge objects when overwriting a generated id with a real id', function () {
        var dataWithoutId = {
            author: {
                firstName: 'John',
                lastName: 'Smith',
                __typename: 'Author',
            },
        };
        var dataWithId = {
            author: {
                firstName: 'John',
                id: '129',
                __typename: 'Author',
            },
        };
        var dataIdFromObject = function (object) {
            if (object.__typename && object.id) {
                return object.__typename + '__' + object.id;
            }
            return undefined;
        };
        var queryWithoutId = graphql_tag_1.default(templateObject_34 || (templateObject_34 = tslib_1.__makeTemplateObject(["\n      query {\n        author {\n          firstName\n          lastName\n          __typename\n        }\n      }\n    "], ["\n      query {\n        author {\n          firstName\n          lastName\n          __typename\n        }\n      }\n    "])));
        var queryWithId = graphql_tag_1.default(templateObject_35 || (templateObject_35 = tslib_1.__makeTemplateObject(["\n      query {\n        author {\n          firstName\n          id\n          __typename\n        }\n      }\n    "], ["\n      query {\n        author {\n          firstName\n          id\n          __typename\n        }\n      }\n    "])));
        var expStoreWithoutId = objectCache_1.defaultNormalizedCacheFactory({
            '$ROOT_QUERY.author': {
                firstName: 'John',
                lastName: 'Smith',
                __typename: 'Author',
            },
            ROOT_QUERY: {
                author: {
                    type: 'id',
                    id: '$ROOT_QUERY.author',
                    generated: true,
                    typename: 'Author',
                },
            },
        });
        var expStoreWithId = objectCache_1.defaultNormalizedCacheFactory({
            Author__129: {
                firstName: 'John',
                lastName: 'Smith',
                id: '129',
                __typename: 'Author',
            },
            ROOT_QUERY: {
                author: {
                    type: 'id',
                    id: 'Author__129',
                    generated: false,
                    typename: 'Author',
                },
            },
        });
        var storeWithoutId = writer.writeQueryToStore({
            result: dataWithoutId,
            query: queryWithoutId,
            dataIdFromObject: dataIdFromObject,
        });
        expect(storeWithoutId.toObject()).toEqual(expStoreWithoutId.toObject());
        var storeWithId = writer.writeQueryToStore({
            result: dataWithId,
            query: queryWithId,
            store: storeWithoutId,
            dataIdFromObject: dataIdFromObject,
        });
        expect(storeWithId.toObject()).toEqual(expStoreWithId.toObject());
    });
    it('should allow a union of objects of a different type, when overwriting a generated id with a real id', function () {
        var dataWithPlaceholder = {
            author: {
                hello: 'Foo',
                __typename: 'Placeholder',
            },
        };
        var dataWithAuthor = {
            author: {
                firstName: 'John',
                lastName: 'Smith',
                id: '129',
                __typename: 'Author',
            },
        };
        var dataIdFromObject = function (object) {
            if (object.__typename && object.id) {
                return object.__typename + '__' + object.id;
            }
            return undefined;
        };
        var query = graphql_tag_1.default(templateObject_36 || (templateObject_36 = tslib_1.__makeTemplateObject(["\n      query {\n        author {\n          ... on Author {\n            firstName\n            lastName\n            id\n            __typename\n          }\n          ... on Placeholder {\n            hello\n            __typename\n          }\n        }\n      }\n    "], ["\n      query {\n        author {\n          ... on Author {\n            firstName\n            lastName\n            id\n            __typename\n          }\n          ... on Placeholder {\n            hello\n            __typename\n          }\n        }\n      }\n    "])));
        var expStoreWithPlaceholder = objectCache_1.defaultNormalizedCacheFactory({
            '$ROOT_QUERY.author': {
                hello: 'Foo',
                __typename: 'Placeholder',
            },
            ROOT_QUERY: {
                author: {
                    type: 'id',
                    id: '$ROOT_QUERY.author',
                    generated: true,
                    typename: 'Placeholder',
                },
            },
        });
        var expStoreWithAuthor = objectCache_1.defaultNormalizedCacheFactory({
            Author__129: {
                firstName: 'John',
                lastName: 'Smith',
                id: '129',
                __typename: 'Author',
            },
            ROOT_QUERY: {
                author: {
                    type: 'id',
                    id: 'Author__129',
                    generated: false,
                    typename: 'Author',
                },
            },
        });
        var store = writer.writeQueryToStore({
            result: dataWithPlaceholder,
            query: query,
            dataIdFromObject: dataIdFromObject,
        });
        expect(store.toObject()).toEqual(expStoreWithPlaceholder.toObject());
        writer.writeQueryToStore({
            result: dataWithAuthor,
            query: query,
            store: store,
            dataIdFromObject: dataIdFromObject,
        });
        expect(store.toObject()).toEqual(expStoreWithAuthor.toObject());
        writer.writeQueryToStore({
            result: dataWithPlaceholder,
            query: query,
            store: store,
            dataIdFromObject: dataIdFromObject,
        });
        expect(store.toObject()).toEqual(tslib_1.__assign(tslib_1.__assign({}, expStoreWithAuthor.toObject()), expStoreWithPlaceholder.toObject()));
    });
    it('does not swallow errors other than field errors', function () {
        var query = graphql_tag_1.default(templateObject_37 || (templateObject_37 = tslib_1.__makeTemplateObject(["\n      query {\n        ...notARealFragment\n        fortuneCookie\n      }\n    "], ["\n      query {\n        ...notARealFragment\n        fortuneCookie\n      }\n    "])));
        var result = {
            fortuneCookie: 'Star Wars unit tests are boring',
        };
        expect(function () {
            writer.writeQueryToStore({
                result: result,
                query: query,
            });
        }).toThrowError(/No fragment/);
    });
    it('does not change object references if the value is the same', function () {
        var query = graphql_tag_1.default(templateObject_38 || (templateObject_38 = tslib_1.__makeTemplateObject(["\n      {\n        id\n        stringField\n        numberField\n        nullField\n      }\n    "], ["\n      {\n        id\n        stringField\n        numberField\n        nullField\n      }\n    "])));
        var result = {
            id: 'abcd',
            stringField: 'This is a string!',
            numberField: 5,
            nullField: null,
        };
        var store = writer.writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
        });
        var newStore = writer.writeQueryToStore({
            query: query,
            result: lodash_1.cloneDeep(result),
            store: objectCache_1.defaultNormalizedCacheFactory(store.toObject()),
        });
        Object.keys(store.toObject()).forEach(function (field) {
            expect(store.get(field)).toEqual(newStore.get(field));
        });
    });
    describe('writeResultToStore shape checking', function () {
        var query = graphql_tag_1.default(templateObject_39 || (templateObject_39 = tslib_1.__makeTemplateObject(["\n      query {\n        todos {\n          id\n          name\n          description\n        }\n      }\n    "], ["\n      query {\n        todos {\n          id\n          name\n          description\n        }\n      }\n    "])));
        it('should write the result data without validating its shape when a fragment matcher is not provided', function () {
            var result = {
                todos: [
                    {
                        id: '1',
                        name: 'Todo 1',
                    },
                ],
            };
            var newStore = writer.writeResultToStore({
                dataId: 'ROOT_QUERY',
                result: result,
                document: query,
                dataIdFromObject: getIdField,
            });
            expect(newStore.get('1')).toEqual(result.todos[0]);
        });
        it('should warn when it receives the wrong data with non-union fragments (using an heuristic matcher)', function () {
            var fragmentMatcherFunction = new __1.HeuristicFragmentMatcher().match;
            var result = {
                todos: [
                    {
                        id: '1',
                        name: 'Todo 1',
                    },
                ],
            };
            return withWarning(function () {
                var newStore = writer.writeResultToStore({
                    dataId: 'ROOT_QUERY',
                    result: result,
                    document: query,
                    dataIdFromObject: getIdField,
                    fragmentMatcherFunction: fragmentMatcherFunction,
                });
                expect(newStore.get('1')).toEqual(result.todos[0]);
            }, /Missing field description/);
        });
        it('should warn when it receives the wrong data inside a fragment (using an introspection matcher)', function () {
            var fragmentMatcherFunction = new __1.IntrospectionFragmentMatcher({
                introspectionQueryResultData: {
                    __schema: {
                        types: [
                            {
                                kind: 'UNION',
                                name: 'Todo',
                                possibleTypes: [
                                    { name: 'ShoppingCartItem' },
                                    { name: 'TaskItem' },
                                ],
                            },
                        ],
                    },
                },
            }).match;
            var queryWithInterface = graphql_tag_1.default(templateObject_40 || (templateObject_40 = tslib_1.__makeTemplateObject(["\n        query {\n          todos {\n            id\n            name\n            description\n            ...TodoFragment\n          }\n        }\n\n        fragment TodoFragment on Todo {\n          ... on ShoppingCartItem {\n            price\n            __typename\n          }\n          ... on TaskItem {\n            date\n            __typename\n          }\n          __typename\n        }\n      "], ["\n        query {\n          todos {\n            id\n            name\n            description\n            ...TodoFragment\n          }\n        }\n\n        fragment TodoFragment on Todo {\n          ... on ShoppingCartItem {\n            price\n            __typename\n          }\n          ... on TaskItem {\n            date\n            __typename\n          }\n          __typename\n        }\n      "])));
            var result = {
                todos: [
                    {
                        id: '1',
                        name: 'Todo 1',
                        description: 'Description 1',
                        __typename: 'ShoppingCartItem',
                    },
                ],
            };
            return withWarning(function () {
                var newStore = writer.writeResultToStore({
                    dataId: 'ROOT_QUERY',
                    result: result,
                    document: queryWithInterface,
                    dataIdFromObject: getIdField,
                    fragmentMatcherFunction: fragmentMatcherFunction,
                });
                expect(newStore.get('1')).toEqual(result.todos[0]);
            }, /Missing field price/);
        });
        it('should warn if a result is missing __typename when required (using an heuristic matcher)', function () {
            var fragmentMatcherFunction = new __1.HeuristicFragmentMatcher().match;
            var result = {
                todos: [
                    {
                        id: '1',
                        name: 'Todo 1',
                        description: 'Description 1',
                    },
                ],
            };
            return withWarning(function () {
                var newStore = writer.writeResultToStore({
                    dataId: 'ROOT_QUERY',
                    result: result,
                    document: apollo_utilities_1.addTypenameToDocument(query),
                    dataIdFromObject: getIdField,
                    fragmentMatcherFunction: fragmentMatcherFunction,
                });
                expect(newStore.get('1')).toEqual(result.todos[0]);
            }, /Missing field __typename/);
        });
        it('should not warn if a field is null', function () {
            var result = {
                todos: null,
            };
            var newStore = writer.writeResultToStore({
                dataId: 'ROOT_QUERY',
                result: result,
                document: query,
                dataIdFromObject: getIdField,
            });
            expect(newStore.get('ROOT_QUERY')).toEqual({ todos: null });
        });
        it('should not warn if a field is defered', function () {
            var originalWarn = console.warn;
            console.warn = jest.fn(function () {
                var args = [];
                for (var _i = 0; _i < arguments.length; _i++) {
                    args[_i] = arguments[_i];
                }
            });
            var defered = graphql_tag_1.default(templateObject_41 || (templateObject_41 = tslib_1.__makeTemplateObject(["\n        query LazyLoad {\n          id\n          expensive @defer\n        }\n      "], ["\n        query LazyLoad {\n          id\n          expensive @defer\n        }\n      "])));
            var result = {
                id: 1,
            };
            var fragmentMatcherFunction = new __1.HeuristicFragmentMatcher().match;
            var newStore = writer.writeResultToStore({
                dataId: 'ROOT_QUERY',
                result: result,
                document: defered,
                dataIdFromObject: getIdField,
                fragmentMatcherFunction: fragmentMatcherFunction,
            });
            expect(newStore.get('ROOT_QUERY')).toEqual({ id: 1 });
            expect(console.warn).not.toBeCalled();
            console.warn = originalWarn;
        });
    });
    it('throws when trying to write an object without id that was previously queried with id', function () {
        var store = objectCache_1.defaultNormalizedCacheFactory({
            ROOT_QUERY: lodash_1.assign({}, {
                __typename: 'Query',
                item: {
                    type: 'id',
                    id: 'abcd',
                    generated: false,
                },
            }),
            abcd: lodash_1.assign({}, {
                id: 'abcd',
                __typename: 'Item',
                stringField: 'This is a string!',
            }),
        });
        expect(function () {
            writer.writeQueryToStore({
                store: store,
                result: {
                    item: {
                        __typename: 'Item',
                        stringField: 'This is still a string!',
                    },
                },
                query: graphql_tag_1.default(templateObject_42 || (templateObject_42 = tslib_1.__makeTemplateObject(["\n          query Failure {\n            item {\n              stringField\n            }\n          }\n        "], ["\n          query Failure {\n            item {\n              stringField\n            }\n          }\n        "]))),
                dataIdFromObject: getIdField,
            });
        }).toThrowErrorMatchingSnapshot();
        expect(function () {
            writer.writeResultToStore({
                store: store,
                result: {
                    item: {
                        __typename: 'Item',
                        stringField: 'This is still a string!',
                    },
                },
                dataId: 'ROOT_QUERY',
                document: graphql_tag_1.default(templateObject_43 || (templateObject_43 = tslib_1.__makeTemplateObject(["\n          query {\n            item {\n              stringField\n            }\n          }\n        "], ["\n          query {\n            item {\n              stringField\n            }\n          }\n        "]))),
                dataIdFromObject: getIdField,
            });
        }).toThrowError(/stringField(.|\n)*abcd/g);
    });
    it('properly handles the connection directive', function () {
        var store = objectCache_1.defaultNormalizedCacheFactory();
        writer.writeQueryToStore({
            query: graphql_tag_1.default(templateObject_44 || (templateObject_44 = tslib_1.__makeTemplateObject(["\n        {\n          books(skip: 0, limit: 2) @connection(key: \"abc\") {\n            name\n          }\n        }\n      "], ["\n        {\n          books(skip: 0, limit: 2) @connection(key: \"abc\") {\n            name\n          }\n        }\n      "]))),
            result: {
                books: [
                    {
                        name: 'abcd',
                    },
                ],
            },
            store: store,
        });
        writer.writeQueryToStore({
            query: graphql_tag_1.default(templateObject_45 || (templateObject_45 = tslib_1.__makeTemplateObject(["\n        {\n          books(skip: 2, limit: 4) @connection(key: \"abc\") {\n            name\n          }\n        }\n      "], ["\n        {\n          books(skip: 2, limit: 4) @connection(key: \"abc\") {\n            name\n          }\n        }\n      "]))),
            result: {
                books: [
                    {
                        name: 'efgh',
                    },
                ],
            },
            store: store,
        });
        expect(store.toObject()).toEqual({
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
    });
    it('should keep reference when type of mixed inlined field changes', function () {
        var store = objectCache_1.defaultNormalizedCacheFactory();
        var query = graphql_tag_1.default(templateObject_46 || (templateObject_46 = tslib_1.__makeTemplateObject(["\n      query {\n        animals {\n          species {\n            name\n          }\n        }\n      }\n    "], ["\n      query {\n        animals {\n          species {\n            name\n          }\n        }\n      }\n    "])));
        writer.writeQueryToStore({
            query: query,
            result: {
                animals: [
                    {
                        __typename: 'Animal',
                        species: {
                            __typename: 'Cat',
                            name: 'cat',
                        },
                    },
                ],
            },
            store: store,
        });
        expect(store.toObject()).toEqual({
            '$ROOT_QUERY.animals.0.species': { name: 'cat' },
            ROOT_QUERY: {
                animals: [
                    {
                        generated: true,
                        id: 'ROOT_QUERY.animals.0',
                        type: 'id',
                        typename: 'Animal',
                    },
                ],
            },
            'ROOT_QUERY.animals.0': {
                species: {
                    generated: true,
                    id: '$ROOT_QUERY.animals.0.species',
                    type: 'id',
                    typename: 'Cat',
                },
            },
        });
        writer.writeQueryToStore({
            query: query,
            result: {
                animals: [
                    {
                        __typename: 'Animal',
                        species: {
                            __typename: 'Dog',
                            name: 'dog',
                        },
                    },
                ],
            },
            store: store,
        });
        expect(store.toObject()).toEqual({
            '$ROOT_QUERY.animals.0.species': { name: 'dog' },
            ROOT_QUERY: {
                animals: [
                    {
                        generated: true,
                        id: 'ROOT_QUERY.animals.0',
                        type: 'id',
                        typename: 'Animal',
                    },
                ],
            },
            'ROOT_QUERY.animals.0': {
                species: {
                    generated: true,
                    id: '$ROOT_QUERY.animals.0.species',
                    type: 'id',
                    typename: 'Dog',
                },
            },
        });
    });
    it('should not keep reference when type of mixed inlined field changes to non-inlined field', function () {
        var store = objectCache_1.defaultNormalizedCacheFactory();
        var dataIdFromObject = function (object) {
            if (object.__typename && object.id) {
                return object.__typename + '__' + object.id;
            }
            return undefined;
        };
        var query = graphql_tag_1.default(templateObject_47 || (templateObject_47 = tslib_1.__makeTemplateObject(["\n      query {\n        animals {\n          species {\n            id\n            name\n          }\n        }\n      }\n    "], ["\n      query {\n        animals {\n          species {\n            id\n            name\n          }\n        }\n      }\n    "])));
        writer.writeQueryToStore({
            query: query,
            result: {
                animals: [
                    {
                        __typename: 'Animal',
                        species: {
                            __typename: 'Cat',
                            name: 'cat',
                        },
                    },
                ],
            },
            dataIdFromObject: dataIdFromObject,
            store: store,
        });
        expect(store.toObject()).toEqual({
            '$ROOT_QUERY.animals.0.species': { name: 'cat' },
            ROOT_QUERY: {
                animals: [
                    {
                        generated: true,
                        id: 'ROOT_QUERY.animals.0',
                        type: 'id',
                        typename: 'Animal',
                    },
                ],
            },
            'ROOT_QUERY.animals.0': {
                species: {
                    generated: true,
                    id: '$ROOT_QUERY.animals.0.species',
                    type: 'id',
                    typename: 'Cat',
                },
            },
        });
        writer.writeQueryToStore({
            query: query,
            result: {
                animals: [
                    {
                        __typename: 'Animal',
                        species: {
                            id: 'dog-species',
                            __typename: 'Dog',
                            name: 'dog',
                        },
                    },
                ],
            },
            dataIdFromObject: dataIdFromObject,
            store: store,
        });
        expect(store.toObject()).toEqual({
            '$ROOT_QUERY.animals.0.species': undefined,
            'Dog__dog-species': {
                id: 'dog-species',
                name: 'dog',
            },
            ROOT_QUERY: {
                animals: [
                    {
                        generated: true,
                        id: 'ROOT_QUERY.animals.0',
                        type: 'id',
                        typename: 'Animal',
                    },
                ],
            },
            'ROOT_QUERY.animals.0': {
                species: {
                    generated: false,
                    id: 'Dog__dog-species',
                    type: 'id',
                    typename: 'Dog',
                },
            },
        });
    });
});
var templateObject_1, templateObject_2, templateObject_3, templateObject_4, templateObject_5, templateObject_6, templateObject_7, templateObject_8, templateObject_9, templateObject_10, templateObject_11, templateObject_12, templateObject_13, templateObject_14, templateObject_15, templateObject_16, templateObject_17, templateObject_18, templateObject_19, templateObject_20, templateObject_21, templateObject_22, templateObject_23, templateObject_24, templateObject_25, templateObject_26, templateObject_27, templateObject_28, templateObject_29, templateObject_30, templateObject_31, templateObject_32, templateObject_33, templateObject_34, templateObject_35, templateObject_36, templateObject_37, templateObject_38, templateObject_39, templateObject_40, templateObject_41, templateObject_42, templateObject_43, templateObject_44, templateObject_45, templateObject_46, templateObject_47;
//# sourceMappingURL=writeToStore.js.map