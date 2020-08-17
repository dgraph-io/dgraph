+++
title = "Slash Quick Start"
[menu.main]
    parent = "slash-graphql"
    weight = 1   
+++

*These are draft docs for Slash GraphQL, which is currently in beta*

Welcome to Slash GraphQL. By now, you should have created your first deployment, and are looking for a schema to test out. Don't worry, we've got you covered.

This example is for todo app that can support multiple users. We just have two types: Tasks and Users.

Here's a schema that works with Slash GraphQL:

```graphql
type Task {
  id: ID!
  title: String! @search(by: [fulltext])
  completed: Boolean! @search
  user: User!
}

type User {
  username: String! @id @search(by: [hash])
  name: String @search(by: [exact])
  tasks: [Task] @hasInverse(field: user)
}
```

Let's paste that into the schema tab of Slash GraphQL and hit submit. You now have a fully functional GraphQL API that allows you to create, query and modify records of these two types.

No, really, that's all; nothing else to do; it's there, serving GraphQL --- let's go use it.

## The Schema

The schema itself was pretty simple. It was just a standard GraphQL schema, with a few directives (like `@search`), which are specific to Slash GraphQL.

The task type has four fields: id, title, completed and the user. The title has the `@search` directive on it, which tells Slash Graphql that this field can be used in full text search queries.

The User type uses the username field as an ID, and we will put the email address into that field.

Let's go ahead and populate some data into this fresh database.

## GraphQL Mutations

If you head over to the API explorer tab, you should see the docs tab, which tells you the queries and mutations that your new database supports. Lets create a bunch of tasks, for a few of our users

```graphql
mutation AddTasks {
  addTask(input: [
    {title: "Create a database", completed: false, user: {username: "your-email@example.com"}},
    {title: "Write A Schema", completed: false, user: {username: "your-email@example.com"}},
    {title: "Put Data In", completed: false, user: {username: "your-email@example.com"}},
    {title: "Complete Tasks with UI", completed: false, user: {username: "your-email@example.com"}},
    {title: "Profit!", completed: false, user: {username: "your-email@example.com"}},

    {title: "Walking", completed: false, user: {username: "frodo@dgraph.io"}},
    {title: "More Walking", completed: false, user: {username: "frodo@dgraph.io"}},
    {title: "Discard Jewelery", completed: false, user: {username: "frodo@dgraph.io"}},

    {title: "Meet Dad", completed: false, user: {username: "skywalker@dgraph.io"}},
    {title: "Dismantle Empire", completed: false, user: {username: "skywalker@dgraph.io"}}
  ]) {
    numUids
    task {
      title
      user {
        username
      }
    }
  }
}
```

Let's also query back the users and their tasks
```graphql
{
  queryUser {
    username,
    tasks {
      title
    }
  }
}
```

You'll see that Slash figured out that users are unique by their username, and so you only see a single record for each user.

## Auth

Now that we have a schema working, let's update that schema to add some authorization. We'll update the schema so that users can only read their own tasks back.

```graphql
type Task @auth(
  query: { rule: """
    query($USER: String!) {
      queryTask {
        user(filter: { username: { eq: $USER } }) {
          __typename
        }
      }
    }"""}), {
  id: ID!
  title: String! @search(by: [fulltext])
  completed: Boolean! @search
  user: User!
}

type User {
  username: String! @id @search(by: [hash])
  name: String @search(by: [exact])
  tasks: [Task] @hasInverse(field: user)
}

# Dgraph.Authorization {"Header":"X-Auth-Token","Namespace":"https://dgraph.io/jwt/claims","Algo":"RS256","Audience":["Q1nC2kLsN6KQTX1UPdiBS6AhXRx9KwKl"],"VerificationKey":"-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAp/qw/KXH23bpOuhXzsDp\ndo9bGNqjd/OkH2LkCT0PKFx5i/lmvFXdd04fhJD0Z0K3pUe7xHcRn1pIbZWlhwOR\n7siaCh9L729OQjnrxU/aPOKwsD19YmLWwTeVpE7vhDejhnRaJ7Pz8GImX/z/Xo50\nPFSYdX28Fb3kssfo+cMBz2+7h1prKeLZyDk30ItK9MMj9S5y+UKHDwfLV/ZHSd8m\nVVEYRXUNNzLsxD2XaEC5ym2gCjEP1QTgago0iw3Bm2rNAMBePgo4OMgYjH9wOOuS\nVnyvHhZdwiZAd1XtJSehORzpErgDuV2ym3mw1G9mrDXDzX9vr5l5CuBc3BjnvcFC\nFwIDAQAB\n-----END PUBLIC KEY-----"}
```

Slash GraphQL allows you to pass JWT with custom claims as a header, and will apply rules to control who can query or modify the data in your database. The `@auth` directive controls how these rules are applied, as filters that are generated from the JWT token.

In our schema, we specify that one can only query tasks if the tasks's user has a username that matches `$USER`, a field in the JWT token.

The Authorization magic comment specifies the header the JWT comes from, the domain, and the key that's signed it. In this example, the key is tied to our dev Auth0 account.

More information on how this works in [the documentation](/graphql/authorization/authorization-overview).

Let's try querying back the tasks. We should be getting empty results here, since you no longer have access.

```graphql
{
  queryTask {
    title
  }
}
```

## Testing it out with a Simple UI

We've built a todo app with react that you can use to close these todos off. Let's head over to our sample react app, deployed at [https://relaxed-brahmagupta-f8020f.netlify.app/](https://relaxed-brahmagupta-f8020f.netlify.app/).

You can try creating an account with your email, or logging in with frodo / skywalker. Like the first death star, Luke wasn't big on security, his password is `password`. Frodo has the same password.
