# graphql-tag
[![npm version](https://badge.fury.io/js/graphql-tag.svg)](https://badge.fury.io/js/graphql-tag)
[![Build Status](https://travis-ci.org/apollographql/graphql-tag.svg?branch=master)](https://travis-ci.org/apollographql/graphql-tag)
[![Get on Slack](https://img.shields.io/badge/slack-join-orange.svg)](http://www.apollodata.com/#slack)

Helpful utilities for parsing GraphQL queries. Includes:

- `gql` A JavaScript template literal tag that parses GraphQL query strings into the standard GraphQL AST.
- `/loader` A webpack loader to preprocess queries

`graphql-tag` uses [the reference `graphql` library](https://github.com/graphql/graphql-js) under the hood as a peer dependency, so in addition to installing this module, you'll also have to install `graphql-js`.

### gql

This is a template literal tag you can use to concisely write a GraphQL query that is parsed into the standard GraphQL AST:

```js
import gql from 'graphql-tag';

const query = gql`
  {
    user(id: 5) {
      firstName
      lastName
    }
  }
`

// query is now a GraphQL syntax tree object
console.log(query);

// {
//   "kind": "Document",
//   "definitions": [
//     {
//       "kind": "OperationDefinition",
//       "operation": "query",
//       "name": null,
//       "variableDefinitions": null,
//       "directives": [],
//       "selectionSet": {
//         "kind": "SelectionSet",
//         "selections": [
//           {
//             "kind": "Field",
//             "alias": null,
//             "name": {
//               "kind": "Name",
//               "value": "user",
//               ...
```

You can easily explore GraphQL ASTs on [astexplorer.net](https://astexplorer.net/#/drYr8X1rnP/1).

This package is the way to pass queries into [Apollo Client](https://github.com/apollographql/apollo-client). If you're building a GraphQL client, you can use it too!

#### Why use this?

GraphQL strings are the right way to write queries in your code, because they can be statically analyzed using tools like [eslint-plugin-graphql](https://github.com/apollographql/eslint-plugin-graphql). However, strings are inconvenient to manipulate, if you are trying to do things like add extra fields, merge multiple queries together, or other interesting stuff.

That's where this package comes in - it lets you write your queries with [ES2015 template literals](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Template_literals) and compile them into an AST with the `gql` tag.

#### Caching parse results

This package only has one feature - it caches previous parse results in a simple dictionary. This means that if you call the tag on the same query multiple times, it doesn't waste time parsing it again. It also means you can use `===` to compare queries to check if they are identical.

### Babel preprocessing

GraphQL queries can be compiled at build time using [babel-plugin-graphql-tag](https://github.com/gajus/babel-plugin-graphql-tag). Pre-compiling queries decreases the script initialization time and reduces the bundle size by potentially removing the need for `graphql-tag` at runtime.

#### TypeScript
Try this custom transformer to pre-compile your GraphQL queries in TypeScript: [ts-transform-graphql-tag](https://github.com/firede/ts-transform-graphql-tag).

#### React Native, Next.js

Additionally, in certain situations, preprocessing queries via the webpack loader is not possible. [babel-plugin-import-graphql](https://www.npmjs.com/package/babel-plugin-import-graphql) will allow one to import graphql files directly into your JavaScript by preprocessing GraphQL queries into ASTs at compile-time.

E.g.:
```javascript
import myImportedQuery from './productsQuery.graphql'

class ProductsPage extends React.Component {
  ...
}
```

#### Create-React-App

`create-react-app@2.0.0` does support the ability to preprocess queries using [evenchange4/graphql.macro](https://github.com/evenchange4/graphql.macro).

If you're using an older version of `create-react-app`, check out [react-app-rewire-inline-import-graphql-ast](https://www.npmjs.com/package/react-app-rewire-inline-import-graphql-ast) to preprocess queries without needing to eject.

### Webpack preprocessing with `graphql-tag/loader`

This package also includes a [webpack loader](https://webpack.js.org/concepts/loaders). There are many benefits over this approach, which saves GraphQL ASTs processing time on client-side and enable queries to be separated from script over `.graphql` files.

```js
loaders: [
  {
    test: /\.(graphql|gql)$/,
    exclude: /node_modules/,
    loader: 'graphql-tag/loader'
  }
]
```

then:

```js
import query from './query.graphql';

console.log(query);
// {
//   "kind": "Document",
// ...
```

Testing environments that don't support Webpack require additional configuration. For [Jest](https://facebook.github.io/jest/) use [jest-transform-graphql](https://github.com/remind101/jest-transform-graphql).

#### Support for multiple operations

With the webpack loader, you can also import operations by name:

In a file called `query.gql`:
```graphql
query MyQuery1 {
  ...
}

query MyQuery2 {
  ...
}
```

And in your JavaScript:
```javascript
import { MyQuery1, MyQuery2 } from 'query.gql'
```

### Warnings

This package will emit a warning if you have multiple fragments of the same name. You can disable this with:

```js
import { disableFragmentWarnings } from 'graphql-tag';

disableFragmentWarnings()
```

### Experimental Fragment Variables

This package exports an `experimentalFragmentVariables` flag that allows you to use experimental support for [parameterized fragments](https://github.com/facebook/graphql/issues/204).

You can enable / disable this with:
```js
import { enableExperimentalFragmentVariables, disableExperimentalFragmentVariables } from 'graphql-tag';
```

Enabling this feature allows you declare documents of the form
```graphql
fragment SomeFragment ($arg: String!) on SomeType {
  someField
}
```
