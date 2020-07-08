(function (global, factory) {
  if (typeof define === "function" && define.amd) {
    define(["exports", "apollo-utilities"], factory);
  } else if (typeof exports !== "undefined") {
    factory(exports, require("apollo-utilities"));
  } else {
    var mod = {
      exports: {}
    };
    factory(mod.exports, global.apolloUtilities);
    global.unknown = mod.exports;
  }
})(typeof globalThis !== "undefined" ? globalThis : typeof self !== "undefined" ? self : this, function (_exports, _apolloUtilities) {

  _exports.__esModule = true;
  _exports.Cache = _exports.ApolloCache = void 0;

  function queryFromPojo(obj) {
    var op = {
      kind: 'OperationDefinition',
      operation: 'query',
      name: {
        kind: 'Name',
        value: 'GeneratedClientQuery'
      },
      selectionSet: selectionSetFromObj(obj)
    };
    var out = {
      kind: 'Document',
      definitions: [op]
    };
    return out;
  }

  function fragmentFromPojo(obj, typename) {
    var frag = {
      kind: 'FragmentDefinition',
      typeCondition: {
        kind: 'NamedType',
        name: {
          kind: 'Name',
          value: typename || '__FakeType'
        }
      },
      name: {
        kind: 'Name',
        value: 'GeneratedClientQuery'
      },
      selectionSet: selectionSetFromObj(obj)
    };
    var out = {
      kind: 'Document',
      definitions: [frag]
    };
    return out;
  }

  function selectionSetFromObj(obj) {
    if (typeof obj === 'number' || typeof obj === 'boolean' || typeof obj === 'string' || typeof obj === 'undefined' || obj === null) {
      return null;
    }

    if (Array.isArray(obj)) {
      return selectionSetFromObj(obj[0]);
    }

    var selections = [];
    Object.keys(obj).forEach(function (key) {
      var nestedSelSet = selectionSetFromObj(obj[key]);
      var field = {
        kind: 'Field',
        name: {
          kind: 'Name',
          value: key
        },
        selectionSet: nestedSelSet || undefined
      };
      selections.push(field);
    });
    var selectionSet = {
      kind: 'SelectionSet',
      selections: selections
    };
    return selectionSet;
  }

  var justTypenameQuery = {
    kind: 'Document',
    definitions: [{
      kind: 'OperationDefinition',
      operation: 'query',
      name: null,
      variableDefinitions: null,
      directives: [],
      selectionSet: {
        kind: 'SelectionSet',
        selections: [{
          kind: 'Field',
          alias: null,
          name: {
            kind: 'Name',
            value: '__typename'
          },
          arguments: [],
          directives: [],
          selectionSet: null
        }]
      }
    }]
  };

  var ApolloCache = function () {
    function ApolloCache() {}

    ApolloCache.prototype.transformDocument = function (document) {
      return document;
    };

    ApolloCache.prototype.transformForLink = function (document) {
      return document;
    };

    ApolloCache.prototype.readQuery = function (options, optimistic) {
      if (optimistic === void 0) {
        optimistic = false;
      }

      return this.read({
        query: options.query,
        variables: options.variables,
        optimistic: optimistic
      });
    };

    ApolloCache.prototype.readFragment = function (options, optimistic) {
      if (optimistic === void 0) {
        optimistic = false;
      }

      return this.read({
        query: (0, _apolloUtilities.getFragmentQueryDocument)(options.fragment, options.fragmentName),
        variables: options.variables,
        rootId: options.id,
        optimistic: optimistic
      });
    };

    ApolloCache.prototype.writeQuery = function (options) {
      this.write({
        dataId: 'ROOT_QUERY',
        result: options.data,
        query: options.query,
        variables: options.variables
      });
    };

    ApolloCache.prototype.writeFragment = function (options) {
      this.write({
        dataId: options.id,
        result: options.data,
        variables: options.variables,
        query: (0, _apolloUtilities.getFragmentQueryDocument)(options.fragment, options.fragmentName)
      });
    };

    ApolloCache.prototype.writeData = function (_a) {
      var id = _a.id,
          data = _a.data;

      if (typeof id !== 'undefined') {
        var typenameResult = null;

        try {
          typenameResult = this.read({
            rootId: id,
            optimistic: false,
            query: justTypenameQuery
          });
        } catch (e) {}

        var __typename = typenameResult && typenameResult.__typename || '__ClientData';

        var dataToWrite = Object.assign({
          __typename: __typename
        }, data);
        this.writeFragment({
          id: id,
          fragment: fragmentFromPojo(dataToWrite, __typename),
          data: dataToWrite
        });
      } else {
        this.writeQuery({
          query: queryFromPojo(data),
          data: data
        });
      }
    };

    return ApolloCache;
  }();

  _exports.ApolloCache = ApolloCache;
  var Cache;
  _exports.Cache = Cache;

  (function (Cache) {})(Cache || (_exports.Cache = Cache = {})); 

});
