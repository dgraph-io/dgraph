# apollo-boost

The fastest, easiest way to get started with Apollo Client!

Apollo Boost is a zero-config way to start using Apollo Client. It includes some sensible defaults, such as our recommended `InMemoryCache` and `HttpLink`, which come configured for you with our recommended settings.

## Quick start

First, install `apollo-boost`. If you don't have `graphql` & `react-apollo` already in your project, please install those too.

```shell
npm i apollo-boost graphql react-apollo -S
```

Next, create your client. Once you create your client, hook it up to your app by passing it to the `ApolloProvider` exported from `react-apollo`.

```js
import React from 'react';
import { render } from 'react-dom';
import ApolloClient from 'apollo-boost';
import { ApolloProvider } from 'react-apollo';

// Pass your GraphQL endpoint to uri
const client = new ApolloClient({ uri: 'https://nx9zvp49q7.lp.gql.zone/graphql' });

const ApolloApp = AppComponent => (
  <ApolloProvider client={client}>
    <AppComponent />
  </ApolloProvider>
);

render(ApolloApp(App), document.getElementById('root'));
```

Awesome! Your ApolloClient is now connected to your app. Let's create our `<App />` component and make our first query:

```js
import React from 'react';
import { gql } from 'apollo-boost';
import { Query } from 'react-apollo';

const GET_MOVIES = gql`
  query {
    movie(id: 1) {
      id
      title
    }
  }
`

const App = () => (
  <Query query={GET_MOVIES}>
    {({ loading, error, data }) => {
      if (loading) return <div>Loading...</div>;
      if (error) return <div>Error :(</div>;

      return (
        <Movie title={data.movie.title} />
      )
    }}
  </Query>
)
```

Time to celebrate! 🎉 You just made your first Query component. The Query component binds your GraphQL query to your UI so Apollo Client can take care of fetching your data, tracking loading & error states, and updating your UI via the `data` prop.

## What's in Apollo Boost

Apollo Boost includes some packages that we think are essential to developing with Apollo Client. Here's what's in the box:

- `apollo-client`: Where all the magic happens
- `apollo-cache-inmemory`: Our recommended cache
- `apollo-link-http`: An Apollo Link for remote data fetching
- `apollo-link-error`: An Apollo Link for error handling
- `graphql-tag`: Exports the `gql` function for your queries & mutations

The awesome thing about Apollo Boost is that you don't have to set any of this up yourself! Just specify a few options if you'd like to use these features and we'll take care of the rest. For a full list of available options, please refer to the Apollo Boost [configuration options](https://www.apollographql.com/docs/react/essentials/get-started.html#configuration-options) documentation.
