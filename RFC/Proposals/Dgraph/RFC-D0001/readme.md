```yaml
  RFC: RFC-D0001.
  Title: GraphQL @auth after-update option
  Author: raphael@dgraph.io
  Co-Author: community
  Status: Accepted

```

## Implement after value test for update opertaion in @auth GraphQL directive

### Summary:
add an 'update-after' config in the @auth directive:

```
type User @auth(
    query: { rule:  "{$<claim>: { eq: \"<value>\" } }" },
    add: { rule:  "{$<claim>: { in: [\"<value1>\",...] } }" },
    update: ...
    update-after: ...
    delete: ...
)
```

For ABAC rules, the operation is comitted only if the object UID is still in the list of authorized objects with the after values.


### Motivation:

As stated in the documentation of v23.0
``Updates have both a before and after state that can be important for authorization.

For example, consider a rule stating that you can only update your own to-do list items. If evaluated in the database before the mutation (like the delete rules) it would prevent you from updating anyone elses to-do list items, but it does not stop you from updating your own to-do items to have a different owner. If evaluated in the database after the mutation occurs, like for add rules, it would prevent setting the owner to another user, but would not prevent editing other’s posts.

Currently, Dgraph evaluates update rules before the mutation.``

Note that, @auth on ``add`` is naturaly executed on the database as-if the add is commited. 

Without a possibility protect the update as-if comitted,then add @auth is less secure,user can simply add a data with valid values and do an update with invalid values.


**Example**
An example that can used as a test

Tasks have a comment and an assignee.
Assignee can only read their task and update their task (to update the comment for example).

In the current situation, user being able to update (assignee), can change the assignee.

The update-after must reject such update, by checking the rule with the ``after`` value.

```
update-after: { rule: """
        query ($USER: String!) {
            queryTask {
                assignee(filter: { username: { eq: $USER } } ) {
                    __typename
                }
            }
        }"""
    }
```

### Abstract Design:

Evaluate the ABAC rules after the update and just before the commit.
Commit only entities that are also present in the ABAC result.

### Technical Details:



### Drawbacks:

Complexify the @auth configuration in the GraphQL Schema but that will be adressed by offering a configuration UI tool.

### Alternatives:

No alternative today for all cases.

### Adoption Strategy:


--------
