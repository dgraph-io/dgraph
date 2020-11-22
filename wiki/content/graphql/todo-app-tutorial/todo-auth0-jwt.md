+++
title = "Using Auth0"
weight = 5
[menu.main]
    parent = "todo-app-tutorial"
+++

Let's start by going to our Auth0 dashboard where we can see the application which we have already created and used in our frontend-application.

![Dashboard](/images/graphql/tutorial/todo/dashboard.png)

Now we want to use the JWT that Auth0 generates, but we also need to add custom claims to that token which will be used by our auth rules.
So we can use something known as "Rules" (left sidebar on dashboard page) to add custom claims to a token. Let's create a new empty rule.

![Rule](/images/graphql/tutorial/todo/rule.png)

Replace the content with the the following -
```javascript
function (user, context, callback) {
  const namespace = "https://dgraph.io/jwt/claims";
  context.idToken[namespace] =
    {
      'USER': user.email,
    };
  
  return callback(null, user, context);
}
```

In the above function, we are only just adding the custom claim to the token with a field as `USER` which if you recall from the last step is used in our auth rules, so it needs to match exactly with that name. 

Now let's go to `Settings` of our Auth0 application and then go down to view the `Advanced Settings` to check the JWT signature algorithm (OAuth tab) and then get the certificate (Certificates tab). We will be using `RS256` in this example so let's make sure it's set to that and then copy the certificate which we will use to get the public key.  Use the download certificate button there to get the certificate in `PEM`. 

![Certificate](/images/graphql/tutorial/todo/certificate.png)

Now let's run a command to get the public key from it, which we will add to our schema. Just change the `file_name` and run the command.

```
openssl x509 -pubkey -noout -in file_name.pem
```

Copy the public key and now let's add it to our schema. For doing that we will add something like this, to the bottom of our schema file - 

```
# Dgraph.Authorization X-Auth0-Token https://dgraph.io/jwt/claims RS256 "<AUTH0-APP-PUBLIC-KEY>"
```

Let me just quickly explain what each thing means in that, so firstly we start the line with a `#  Dgraph.Authorization`, next is the name of the header `X-Auth0-Token` (can be anything) which will be used to send the value of the JWT. Next is the custom-claim name `https://dgraph.io/jwt/claims` (again can be anything, just needs to match with the name specified in Auth0). Then next is the `RS256` the JWT signature algorithm (another option is `HS256` but remember to use the same algorithm in Auth0) and lastly, update `<AUTH0-APP-PUBLIC-KEY>` with your public key within the quotes and make sure to have it in a single line and add `\n` where ever needed.  The updated schema will look something like this (update the public key with your key) - 

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
  name: String
  tasks: [Task] @hasInverse(field: user)
}
# Dgraph.Authorization X-Auth0-Token https://dgraph.io/jwt/claims RS256 "<AUTH0-APP-PUBLIC-KEY>"
```

Resubmit the updated schema -
```
curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
```

Let's get that token and see what all it contains, then update the frontend accordingly. For doing this, let's start our app again.

```
npm start
```

Now open a browser window, navigate to [http://localhost:3000](http://localhost:3000) and open the developer tools, go to the `network` tab and find a call called `token` to get your JWT from its response JSON (field `id_token`).

![Token](/images/graphql/tutorial/todo/token.png)

Now go to [jwt.io](https://jwt.io) and paste your token there.

![jwt](/images/graphql/tutorial/todo/jwt.png)

The token also includes our custom claim like below.

```json
{
"https://dgraph.io/jwt/claims": {
    "USER": "vardhanapoorv"
  },
  ...
}
  ```

Now, you can check if the auth rule that we added is working as expected or not. Open the GraphQL tool (Insomnia, GraphQL Playground) add the URL along with the header `X-Auth0-Token` and its value as the JWT. Let's try the query to see the todos and only the todos the logged-in user created should be visible.
```graphql
query {
  queryTask {
    title
    completed
    user {
        username
    }
  }
}
```

The above should give you only your todos and verifies that our auth rule worked!

Now let's update our frontend application to include the `X-Auth0-Token` header with value as JWT from Auth0 when sending a  request.

To do this, we need to update the Apollo client setup to include the header while sending the request, and we need to get the JWT from Auth0. 

The value we want is in the field `idToken` from Auth0. We get that by quickly updating `react-auth0-spa.js` to get `idToken` and pass it as a prop to our `App`.

```javascript
...

const [popupOpen, setPopupOpen] = useState(false);
const [idToken, setIdToken] = useState("");

...

if (isAuthenticated) {
        const user = await auth0FromHook.getUser();
        setUser(user);
        const idTokenClaims = await auth0FromHook.getIdTokenClaims();
        setIdToken(idTokenClaims.__raw);
}

...

const user = await auth0Client.getUser();
const idTokenClaims = await auth0Client.getIdTokenClaims();

setIdToken(idTokenClaims.__raw);

...

{children}
      <App idToken={idToken} />
    </Auth0Context.Provider>

...

```

Check the updated file [here](https://github.com/dgraph-io/graphql-sample-apps/blob/c94b6eb1cec051238b81482a049100b1cd15bbf7/todo-app-react/src/react-auth0-spa.js)

 Now let's use that token while creating an Apollo client instance and give it to a header `X-Auth0-Token` in our case.  Let's update our `src/App.js` file.

```javascript
...

import { useAuth0 } from "./react-auth0-spa";
import { setContext } from "apollo-link-context";

// Updated to take token
const createApolloClient = token => {
  const httpLink = createHttpLink({
    uri: config.graphqlUrl,
    options: {
      reconnect: true,
    },
});

// Add header
const authLink = setContext((_, { headers }) => {
    // return the headers to the context so httpLink can read them
    return {
      headers: {
        ...headers,
        "X-Auth-Token": token,
      },
    };
});

// Include header
return new ApolloClient({
    link: httpLink,
    link: authLink.concat(httpLink),
    cache: new InMemoryCache()
});

// Get token from props and pass to function
const App = ({idToken}) => {
  const { loading } = useAuth0();
  if (loading) {
    return <div>Loading...</div>;
  }
const client = createApolloClient(idToken);

...
```

Check the updated file [here](https://github.com/dgraph-io/graphql-sample-apps/blob/c94b6eb1cec051238b81482a049100b1cd15bbf7/todo-app-react/src/App.js).

Refer this step in [GitHub](https://github.com/dgraph-io/graphql-sample-apps/commit/c94b6eb1cec051238b81482a049100b1cd15bbf7).

Let's now start the app.

```
npm start
```

Now you should have an app running with Auth0!
