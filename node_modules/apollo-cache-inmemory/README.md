---
title: InMemoryCache
description: An explanation of `apollo-cache-inmemory`
---

`apollo-cache-inmemory` is the recommended cache implementation for Apollo Client 2.0. `InMemoryCache` is a normalized data store that supports all of Apollo Client 1.0's features without the dependency on Redux.

In some instances, you may need to manipulate the cache directly, such as updating the store after a mutation. We'll cover some common use cases [here](#recipes).

<h2 id="installation">Installation</h2>

```
npm install apollo-cache-inmemory --save
```

After installing the package, you'll want to initialize the cache constructor. Then, you can pass in your newly created cache to ApolloClient.

```js
import { InMemoryCache } from 'apollo-cache-inmemory';
import { HttpLink } from 'apollo-link-http';
import ApolloClient from 'apollo-client';

const cache = new InMemoryCache();

const client = new ApolloClient({
  link: new HttpLink(),
  cache
});
```

<h2 id="configuration">Configuration</h2>

The `InMemoryCache` constructor takes an optional config object with properties to customize your cache:

- addTypename: A boolean to determine whether to add __typename to the document (default: `true`)
- dataIdFromObject: A function that takes a data object and returns a unique identifier to be used when normalizing the data in the store. Learn more about how to customize `dataIdFromObject` in the [Normalization](#normalization) section.
- fragmentMatcher: By default, the `InMemoryCache` uses a heuristic fragment matcher. If you are using fragments on unions and interfaces, you will need to use an `IntrospectionFragmentMatcher`. For more information, please read [our guide to setting up fragment matching for unions & interfaces](https://www.apollographql.com/docs/react/advanced/fragments.html#fragment-matcher).

<h2 id="normalization">Normalization</h2>

The `InMemoryCache` normalizes your data before saving it to the store by splitting the result into individual objects, creating a unique identifier for each object, and storing those objects in a flattened data structure. By default, `InMemoryCache` will attempt to use the commonly found primary keys of `id` and `_id` for the unique identifier if they exist along with `__typename` on an object.

If `id` and `_id` are not specified, or if `__typename` is not specified, `InMemoryCache` will fall back to the path to the object in the query, such as `ROOT_QUERY.allPeople.0` for the first record returned on the `allPeople` root query.

This "getter" behavior for unique identifiers can be configured manually via the `dataIdFromObject` option passed to the `InMemoryCache` constructor, so you can pick which field is used if some of your data follows unorthodox primary key conventions.

For example, if you wanted to key off of the `key` field for all of your data, you could configure `dataIdFromObject` like so:

```js
const cache = new InMemoryCache({
  dataIdFromObject: object => object.key
});
```

This also allows you to use different unique identifiers for different data types by keying off of the `__typename` property attached to every object typed by GraphQL.  For example:

```js
const cache = new InMemoryCache({
  dataIdFromObject: object => {
    switch (object.__typename) {
      case 'foo': return object.key; // use `key` as the primary key
      case 'bar': return object.blah; // use `blah` as the priamry key
      default: return object.id || object._id; // fall back to `id` and `_id` for all other types
    }
  }
});
```

<h2 id="direct">Direct Cache Access</h2>

To interact directly with your cache, you can use the Apollo Client class methods readQuery, readFragment, writeQuery, and writeFragment. These methods are available to us via the [`DataProxy` interface](https://github.com/apollographql/apollo-client/blob/master/packages/apollo-cache/src/types/DataProxy.ts). Accessing these methods will vary slightly based on your view layer implementation. If you are using React, you can wrap your component in the `withApollo` higher order component, which will give you access to `this.props.client`. From there, you can use the methods to control your data.

Any code demonstration in the following sections will assume that we have already initialized an instance of  `ApolloClient` and that we have imported the `gql` tag from `graphql-tag`.

<h3 id="readquery">readQuery</h3>

The `readQuery` method is very similar to the [`query` method on `ApolloClient`][] except that `readQuery` will _never_ make a request to your GraphQL server. The `query` method, on the other hand, may send a request to your server if the appropriate data is not in your cache whereas `readQuery` will throw an error if the data is not in your cache. `readQuery` will _always_ read from the cache. You can use `readQuery` by giving it a GraphQL query like so:

```js
const { todo } = client.readQuery({
  query: gql`
    query ReadTodo {
      todo(id: 5) {
        id
        text
        completed
      }
    }
  `,
});
```

If all of the data needed to fulfill this read is in Apollo Client’s normalized data cache then a data object will be returned in the shape of the query you wanted to read. If not all of the data needed to fulfill this read is in Apollo Client’s cache then an error will be thrown instead, so make sure to only read data that you know you have!

You can also pass variables into `readQuery`.

```js
const { todo } = client.readQuery({
  query: gql`
    query ReadTodo($id: Int!) {
      todo(id: $id) {
        id
        text
        completed
      }
    }
  `,
  variables: {
    id: 5,
  },
});
```

<h3 id="readfragment">readFragment</h3>

This method allows you great flexibility around the data in your cache. Whereas `readQuery` only allowed you to read data from your root query type, `readFragment` allows you to read data from _any node you have queried_. This is incredibly powerful. You use this method as follows:

```js
const todo = client.readFragment({
  id: ..., // `id` is any id that could be returned by `dataIdFromObject`.
  fragment: gql`
    fragment myTodo on Todo {
      id
      text
      completed
    }
  `,
});
```

The first argument is the id of the data you want to read from the cache. That id must be a value that was returned by the `dataIdFromObject` function you defined when initializing `ApolloClient`. So for example if you initialized `ApolloClient` like so:

```js
const client = new ApolloClient({
  ...,
  dataIdFromObject: object => object.id,
});
```

…and you requested a todo before with an id of `5`, then you can read that todo out of your cache with the following:

```js
const todo = client.readFragment({
  id: '5',
  fragment: gql`
    fragment myTodo on Todo {
      id
      text
      completed
    }
  `,
});
```

> **Note:** Most people add a `__typename` to the id in `dataIdFromObject`. If you do this then don’t forget to add the `__typename` when you are reading a fragment as well. So for example your id may be `Todo_5` and not just `5`.

If a todo with that id does not exist in the cache you will get `null` back. If a todo of that id does exist in the cache, but that todo does not have the `text` field then an error will be thrown.

The beauty of `readFragment` is that the todo could have come from anywhere! The todo could have been selected as a singleton (`{ todo(id: 5) { ... } }`), the todo could have come from a list of todos (`{ todos { ... } }`), or the todo could have come from a mutation (`mutation { createTodo { ... } }`). As long as at some point your GraphQL server gave you a todo with the provided id and fields `id`, `text`, and `completed` you can read it from the cache at any part of your code.

<h3 id="writequery-and-writefragment">writeQuery` and `writeFragment</h3>

Not only can you read arbitrary data from the Apollo Client cache, but you can also write any data that you would like to the cache. The methods you use to do this are `writeQuery` and `writeFragment`. They will allow you to change data in your local cache, but it is important to remember that *they will not change any data on your server*. If you reload your environment then changes made with `writeQuery` and `writeFragment` will disappear.

These methods have the same signature as their `readQuery` and `readFragment` counterparts except they also require an additional `data` variable. So for example, if you wanted to update the `completed` flag locally for your todo with id `'5'` you could execute the following:

```js
client.writeFragment({
  id: '5',
  fragment: gql`
    fragment myTodo on Todo {
      completed
    }
  `,
  data: {
    completed: true,
  },
});
```

Any subscriber to the Apollo Client store will instantly see this update and render new UI accordingly.

> **Note:** Again, remember that using `writeQuery` or `writeFragment` only changes data *locally*. If you reload your environment then changes made with these methods will no longer exist.

Or if you wanted to add a new todo to a list fetched from the server, you could use `readQuery` and `writeQuery` together.

```js
const query = gql`
  query MyTodoAppQuery {
    todos {
      id
      text
      completed
    }
  }
`;

const data = client.readQuery({ query });

const myNewTodo = {
  id: '6',
  text: 'Start using Apollo Client.',
  completed: false,
};

client.writeQuery({
  query,
  data: {
    todos: [...data.todos, myNewTodo],
  },
});
```

<h2 id="recipes">Recipes</h2>

Here are some common situations where you would need to access the cache directly. If you're manipulating the cache in an interesting way and would like your example to be featured, please send in a pull request!

<h3 id="server">Server side rendering</h3>

First, you will need to initialize an `InMemoryCache` on the server and create an instance of `ApolloClient`. In the initial serialized HTML payload from the server, you should include a script tag that extracts the data from the cache.

```js
`<script>
  window.__APOLLO_STATE__=${JSON.stringify(cache.extract())}
</script>`
```

On the client, you can rehydrate the cache using the initial data passed from the server:

```js
cache: new Cache().restore(window.__APOLLO_STATE__)
```

If you would like to learn more about server side rendering, please check our our more in depth guide [here].
<!---
TODO (PEGGY): Add link to SSR
-->

<h3 id="server">Updating the cache after a mutation</h3>

Being able to read and write to the Apollo cache from anywhere in your application gives you a lot of power over your data. However, there is one place where we most often want to update our cached data: after a mutation. As such, Apollo Client has optimized the experience for updating your cache with the read and write methods after a mutation with the `update` function. Let us say that we have the following GraphQL mutation:

```graphql
mutation TodoCreateMutation($text: String!) {
  createTodo(text: $text) {
    id
    text
    completed
  }
}
```

We may also have the following GraphQL query:

```graphql
query TodoAppQuery {
  todos {
    id
    text
    completed
  }
}
```

At the end of our mutation we want our query to include the new todo like we had sent our `TodoAppQuery` a second time after the mutation finished without actually sending the query. To do this we can use the `update` function provided as an option of the `client.mutate` method. To update your cache with the mutation just write code that looks like:

```js
// We assume that the GraphQL operations `TodoCreateMutation` and
// `TodoAppQuery` have already been defined using the `gql` tag.

const text = 'Hello, world!';

client.mutate({
  mutation: TodoCreateMutation,
  variables: {
    text,
  },
  update: (proxy, { data: { createTodo } }) => {
    // Read the data from our cache for this query.
    const data = proxy.readQuery({ query: TodoAppQuery });

    // Add our todo from the mutation to the end.
    data.todos.push(createTodo);

    // Write our data back to the cache.
    proxy.writeQuery({ query: TodoAppQuery, data });
  },
});
```

The first `proxy` argument is an instance of [`DataProxy`][] has the same four methods that we just learned exist on the Apollo Client: `readQuery`, `readFragment`, `writeQuery`, and `writeFragment`. The reason we call them on a `proxy` object here instead of on our `client` instance is that we can easily apply optimistic updates (which we will demonstrate in a bit). The `proxy` object also provides an isolated transaction which shields you from any other mutations going on at the same time, and the `proxy` object also batches writes together until the very end.

If you provide an `optimisticResponse` option to the mutation then the `update` function will be run twice. Once immediately after you call `client.mutate` with the data from `optimisticResponse`. After the mutation successfully executes against the server the changes made in the first call to `update` will be rolled back and `update` will be called with the *actual* data returned by the mutation and not just the optimistic response.

Putting it all together:

```js
const text = 'Hello, world!';

client.mutate({
  mutation: TodoCreateMutation,
  variables: {
    text,
  },
  optimisticResponse: {
    id: -1, // -1 is a temporary id for the optimistic response.
    text,
    completed: false,
  },
  update: (proxy, { data: { createTodo } }) => {
    const data = proxy.readQuery({ query: TodoAppQuery });
    data.todos.push(createTodo);
    proxy.writeQuery({ query: TodoAppQuery, data });
  },
});
```

As you can see the `update` function on `client.mutate` provides extra change management functionality specific to the use case of a mutation while still providing you the powerful data control APIs that are available on `client`.

The `update` function is not a good place for side-effects as it may be called multiple times. Also, you may not call any of the methods on `proxy` asynchronously.

