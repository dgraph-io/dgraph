# [Apollo Client](https://www.apollographql.com/client/) [![npm version](https://badge.fury.io/js/apollo-client.svg)](https://badge.fury.io/js/apollo-client) [![Open Source Helpers](https://www.codetriage.com/apollographql/apollo-client/badges/users.svg)](https://www.codetriage.com/apollographql/apollo-client) [![Join the community on Spectrum](https://withspectrum.github.io/badge/badge.svg)](https://spectrum.chat/apollo)

Apollo Client is a fully-featured caching GraphQL client with integrations for React, Angular, and more. It allows you to easily build UI components that fetch data via GraphQL. To get the most value out of `apollo-client`, you should use it with one of its view layer integrations.

To get started with the React integration, go to our [**React Apollo documentation website**](https://www.apollographql.com/docs/react/).

Apollo Client also has view layer integrations for [all the popular frontend frameworks](#learn-how-to-use-apollo-client-with-your-favorite-framework). For the best experience, make sure to use the view integration layer for your frontend framework of choice.

Apollo Client can be used in any JavaScript frontend where you want to use data from a GraphQL server. It's:

1. **Incrementally adoptable**, so that you can drop it into an existing JavaScript app and start using GraphQL for just part of your UI.
2. **Universally compatible**, so that Apollo works with any build setup, any GraphQL server, and any GraphQL schema.
3. **Simple to get started with**, so you can start loading data right away and learn about advanced features later.
4. **Inspectable and understandable**, so that you can have great developer tools to understand exactly what is happening in your app.
5. **Built for interactive apps**, so your users can make changes and see them reflected in the UI immediately.
6. **Small and flexible**, so you don't get stuff you don't need. The core is under 25kb compressed.
7. **Community driven**, because Apollo is driven by the community and serves a variety of use cases. Everything is planned and developed in the open.

Get started on the [home page](http://apollographql.com/client), which has great examples for a variety of frameworks.

## Installation

```bash
# installing the preset package
npm install apollo-boost graphql-tag graphql --save
# installing each piece independently
npm install apollo-client apollo-cache-inmemory apollo-link-http graphql-tag graphql --save
```

To use this client in a web browser or mobile app, you'll need a build system capable of loading NPM packages on the client. Some common choices include Browserify, Webpack, and Meteor 1.3+.

Install the [Apollo Client Developer tools for Chrome](https://chrome.google.com/webstore/detail/apollo-client-developer-t/jdkknkkbebbapilgoeccciglkfbmbnfm) for a great GraphQL developer experience!

## Usage

You get started by constructing an instance of the core class [`ApolloClient`][]. If you load `ApolloClient` from the [`apollo-boost`][] package, it will be configured with a few reasonable defaults such as our standard in-memory cache and a link to a GraphQL API at `/graphql`.

```js
import ApolloClient from 'apollo-boost';

const client = new ApolloClient();
```


To point `ApolloClient` at a different URL, add your GraphQL API's URL to the `uri` config property:

```js
import ApolloClient from 'apollo-boost';

const client = new ApolloClient({
  uri: 'https://graphql.example.com'
});
```

Most of the time you'll hook up your client to a frontend integration. But if you'd like to directly execute a query with your client, you may now call the `client.query` method like this:

```js
import gql from 'graphql-tag';

client.query({
  query: gql`
    query TodoApp {
      todos {
        id
        text
        completed
      }
    }
  `,
})
  .then(data => console.log(data))
  .catch(error => console.error(error));
```

Now your client will be primed with some data in its cache. You can continue to make queries, or you can get your `client` instance to perform all sorts of advanced tasks on your GraphQL data. Such as [reactively watching queries with `watchQuery`][], [changing data on your server with `mutate`][], or [reading a fragment from your local cache with `readFragment`][].

To learn more about all of the features available to you through the `apollo-client` package, be sure to read through the [**`apollo-client` API reference**](https://www.apollographql.com/docs/react/api/apollo-client.html).

[`ApolloClient`]: https://www.apollographql.com/docs/react/api/apollo-client.html
[`apollo-boost`]: https://www.apollographql.com/docs/react/essentials/get-started.html#apollo-boost
[reactively watching queries with `watchQuery`]: https://www.apollographql.com/docs/react/api/apollo-client.html#ApolloClient.watchQuery
[changing data on your server with `mutate`]: https://www.apollographql.com/docs/react/essentials/mutations.html
[reading a fragment from your local cache with `readFragment`]: https://www.apollographql.com/docs/react/advanced/caching.html#direct

## Learn how to use Apollo Client with your favorite framework

- [React](http://apollographql.com/docs/react/)
- [Angular](http://apollographql.com/docs/angular/)
- [Vue](https://github.com/Akryum/vue-apollo)
- [Ember](https://github.com/bgentry/ember-apollo-client)
- [Web Components](https://github.com/apollo-elements/apollo-elements)
- [Meteor](http://apollographql.com/docs/react/recipes/meteor.html)
- [Blaze](http://github.com/Swydo/blaze-apollo)
- [Vanilla JS](https://www.apollographql.com/docs/react/api/apollo-client.html)
- [Next.js](https://github.com/zeit/next.js/tree/master/examples/with-apollo)

---

## Contributing

[![CircleCI](https://circleci.com/gh/apollographql/apollo-client.svg?style=svg)](https://circleci.com/gh/apollographql/apollo-client)
[![codecov](https://codecov.io/gh/apollographql/apollo-client/branch/master/graph/badge.svg)](https://codecov.io/gh/apollographql/apollo-client)

[Read the Apollo Contributor Guidelines.](CONTRIBUTING.md)

Running tests locally:

```
npm install
npm test
```

This project uses TypeScript for static typing and TSLint for linting. You can get both of these built into your editor with no configuration by opening this project in [Visual Studio Code](https://code.visualstudio.com/), an open source IDE which is available for free on all platforms.

#### Important discussions

If you're getting booted up as a contributor, here are some discussions you should take a look at:

1. [Static typing and why we went with TypeScript](https://github.com/apollostack/apollo-client/issues/6) also covered in [the Medium post](https://medium.com/apollo-stack/javascript-code-quality-with-free-tools-9a6d80e29f2d#.k32z401au)
1. [Idea for pagination handling](https://github.com/apollostack/apollo-client/issues/26)
1. [Discussion about interaction with Redux and domain vs. client state](https://github.com/apollostack/apollo-client/issues/98)
1. [Long conversation about different client options, before this repo existed](https://github.com/apollostack/apollo/issues/1)

## Maintainers

- [@benjamn](https://github.com/benjamn) (Apollo)
- [@hwillson](https://github.com/hwillson) (Apollo)
