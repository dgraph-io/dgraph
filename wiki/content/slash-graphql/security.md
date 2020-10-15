+++
title = "Securing Your GraphQL endpoint"
weight = 6
[menu.main]
    parent = "slash-graphql"
+++

Here are a few tips for securing your Slash GraphQL Backend

### Writing Auth Rules

All GraphQL queries and mutations are unrestricted by default. In order to restrict access, please see the [the @auth directive](https://dgraph.io/docs/graphql/authorization/directive/).

### Restricting CORS from allowlisted domains

Restricting the origins that your Slash GraphQL responds to is a an important step in preventing XSS exploits. Your Slash GraphQL backend will prevent any origins that are not in the allowlist from accessing your GraphQL endpoint.

In order to add origins to the allow list, please see the [settings page](https://slash.dgraph.io/_/settings), under the "CORS" tab. By default, we allow all origins to connect to your endpoint (`Access-Control-Allow-Origin: *`), and adding an origin will prevent this default behavior. On adding your first origin, we automatically add "https://slash.dgraph.io"  as well, so that the API explorer continues to work.

Note: CORS restrictions are not a replacement for writing auth rules, as it is possible for malicious actors to bypass these restrictions. Also note that the CORS restrictions only applies to
