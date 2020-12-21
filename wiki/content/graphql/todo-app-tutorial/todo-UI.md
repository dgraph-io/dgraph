+++
title = "Creating a basic UI"
weight = 3
[menu.main]
    parent = "todo-app-tutorial"
+++

In this step, we will create a simple todo app (React) and integrate it with Auth0.

## Create React app

Let's start by creating a React app using the `create-react-app` command.

```
npx create-react-app todo-react-app
```

To verify navigate to the folder, start the dev server, and visit [http://localhost:3000](http://localhost:3000).

```
cd todo-react-app
npm start
```

Refer this step in [GitHub](https://github.com/dgraph-io/graphql-sample-apps/commit/bc235fda6e7557fc9204dd886c67f7eec7bdcadb).

## Install dependencies

Now, let's install the various dependencies that we will need in the app.

```
npm install todomvc-app-css classnames graphql-tag history react-router-dom
```

Refer this step in [GitHub](https://github.com/dgraph-io/graphql-sample-apps/commit/fc7ed70fdde368179e9d7310202b1a0952d2c5c1).

## Setup Apollo Client

Let's start with installing the Apollo dependencies and then create a setup.

```
npm install @apollo/react-hooks apollo-cache-inmemory apollo-client apollo-link-http graphql apollo-link-context
```

Now, let's update our `src/App.js` with the below content to include the Apollo client setup.

```javascript
import React from "react"

import ApolloClient from "apollo-client"
import { InMemoryCache } from "apollo-cache-inmemory"
import { ApolloProvider } from "@apollo/react-hooks"
import { createHttpLink } from "apollo-link-http"

import "./App.css"

const createApolloClient = () => {
  const httpLink = createHttpLink({
    uri: "http://localhost:8080/graphql",
    options: {
      reconnect: true,
    },
  })

  return new ApolloClient({
    link: httpLink,
    cache: new InMemoryCache(),
  })
}

const App = () => {
  const client = createApolloClient()
  return (
    <ApolloProvider client={client}>
      <div>
        <h1>todos</h1>
        <input
          className="new-todo"
          placeholder="What needs to be done?"
          autoFocus={true}
        />
      </div>
    </ApolloProvider>
  )
}

export default App
```

Here we have created a simple instance of the Apollo client and passed the URL of our GraphQL API. Then we have passed the client to `ApolloProvider` and wrapped our `App` so that its accessible throughout the app.

Refer this step in [GitHub](https://github.com/dgraph-io/graphql-sample-apps/commit/f3fedc663e75d2f8ce933432b15db5d5d080ccc2).

## Queries and Mutations

Now, let's add some queries and mutations.

First, let's see how we can add a todo and get todos. Create a file `src/GraphQLData.js` and add the following.

```javascript
import gql from "graphql-tag";

export const ADD_TODO = gql`
  mutation addTask($task: [AddTaskInput!]!) {
    addTask(input: $task) {
      task {
        id
        title
      }
    }
  }
`
export const GET_TODOS = gql`
  query {
    queryTask {
      id
      title
      completed
    }
  }
`
```

Refer to the complete set of queries and mutations [here](https://github.com/dgraph-io/graphql-sample-apps/blob/948e9a8626b1f0c1e40de02485a1110b45f53b89/todo-app-react/src/GraphQLData.js).

Now, let's see how to use that to add a todo.
Let's import the dependencies first in `src/TodoApp.js`

```javascript
import { useQuery, useMutation } from "@apollo/react-hooks"
import { GET_TODOS, ADD_TODO } from "./GraphQLData"
```

Let's now create the functions to add a todo and get todos.

```javascript
const TodoApp = () => {

...
const [addTodo] = useMutation(ADD_TODO);

const { loading, error, data } = useQuery(GET_TODOS);
  const getData = () => {
    if (loading) {
      return null;
    }
    if (error) {
      console.error(`GET_TODOS error: ${error}`);
      return `Error: ${error.message}`;
    }
    if (data.queryTask) {
      setShownTodos(data.queryTask)
    }
  }

 ...

const add = (title) =>
    addTodo({
      variables: { task: [
        { title: title, completed: false, user: { username: "email@example.com" } }
      ]},
      refetchQueries: [{
        query: GET_TODOS
      }]
    });
 ...

```

Refer the complete set of functions [here](https://github.com/dgraph-io/graphql-sample-apps/blob/948e9a8626b1f0c1e40de02485a1110b45f53b89/todo-app-react/src/TodoApp.js).

Also, check the other files updated in this step and make those changes as well.

Refer this step in [GitHub](https://github.com/dgraph-io/graphql-sample-apps/commit/948e9a8626b1f0c1e40de02485a1110b45f53b89).

## Auth0 Integration

Now, let's integrate Auth0 in our application and use that to add the logged-in user. Let's first create an app in Auth0.

- Head over to Auth0 and create an account. Click 'sign up' [here](https://auth0.com/)
- Once the signup is done, click "Create Application" in "Integrate Auth0 into your application".
- Give your app a name and select "Single Page Web App" application type
- Select React as the technology
- No need to do the sample app, scroll down to "Configure Auth0" and select "Application Settings".
- Select your app and add the values of `domain` and `clientid` in the file `src/auth_template.json`. Check this [link](https://auth0.com/docs/quickstart/spa/react/01-login#configure-auth0) for more information.
- Add `http://localhost:3000` to "Allowed Callback URLs", "Allowed Web Origins" and "Allowed Logout URLs".

Check the commit [here](https://github.com/dgraph-io/graphql-sample-apps/commit/4c9c42e1ae64545cb10a24922623a196288d061c) for verifying the Auth0 setup you did after following the above steps.

Let's also add definitions for getting a user and adding it to `src/GraphQLData.js`.

```javascript
import gql from "graphql-tag";

export const GET_USER = gql`
  query getUser($username: String!) {
    getUser(username: $username) {
      username
      name
      tasks {
        id
        title
        completed
      }
    }
  }
`

export const ADD_USER = gql`
  mutation addUser($user: AddUserInput!) {
    addUser(input: [$user]) {
      user {
        username
      }
    }
  }
`
```

Check the updated file [here](https://github.com/dgraph-io/graphql-sample-apps/blob/4c9c42e1ae64545cb10a24922623a196288d061c/todo-app-react/src/GraphQLData.js)

Now, let's also add functions for these in `src/TodoApp.js`.

```javascript
...
import { GET_USER, GET_TODOS, ADD_USER, ADD_TODO, DELETE_TODO, TOGGLE_TODO, UPDATE_TODO, CLEAR_COMPLETED_TODO, TOGGLE_ALL_TODO } from "./GraphQLData";
import { useAuth0 } from "./react-auth0-spa";

...

const useImperativeQuery = (query) => {
  const { refetch } = useQuery(query, { skip: true });
  const imperativelyCallQuery = (variables) => {
    return refetch(variables);
  };
  return imperativelyCallQuery;
};

const TodoApp = () => {

  ...
  const [newTodo, setNewTodo] = useState("");
  const [shownTodos, setShownTodos] = useState([]);

  const [addUser] = useMutation(ADD_USER);

  ...

  const [updateTodo] = useMutation(UPDATE_TODO);
  const [clearCompletedTodo] = useMutation(CLEAR_COMPLETED_TODO);   
  const getUsers = useImperativeQuery(GET_USER)

  const { user } = useAuth0();

  const createUser = () => {
    if (user === undefined) {
      return null;
    }
    const { data: getUser } = getUsers({
      username: user.email
    });
    if (getUser && getUser.getUser === null) {
      const newUser = {
        username: user.email,
        name: user.nickname,
      };
      addUser({
        variables: {
          user: newUser
        }
      })
    }
  }
}

...

```

Check all the changes for the file [here](https://github.com/dgraph-io/graphql-sample-apps/blob/4c9c42e1ae64545cb10a24922623a196288d061c/todo-app-react/src/TodoApp.js)

Let's create a short profile page to display user details. Add files `src/Profile.js` and `src/Profile.css`.

```javascript
import React from "react";
import { useAuth0 } from "./react-auth0-spa";
import './Profile.css';

const Profile = () => {
  const { loading, user } = useAuth0();

  if (loading || !user) {
    return <div>Loading...</div>;
  }

  return (
      <div className="profile">
        <img className="profile-img" src={user.picture} alt="Profile" />
        <p>Name: <strong>{user.nickname}</strong></p>
        <p>Email: <strong>{user.email}</strong></p>
      </div>
  );
};

export default Profile;
```

```css
.profile {
    padding: 15px;
}
.profile-img {
    display: block;
    margin: 0 auto;
    border-radius: 50%;
}
```

Also, check the other files updated in this step and make those changes as well.

Refer this step in [GitHub](https://github.com/dgraph-io/graphql-sample-apps/commit/4c9c42e1ae64545cb10a24922623a196288d061c).

Let's now start the app.

```
npm start
```

Now you should have an app running!
