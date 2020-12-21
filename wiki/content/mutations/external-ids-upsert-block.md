+++
date = "2017-03-20T22:25:17+11:00"
title = "External IDs and Upsert Block"
weight = 4
[menu.main]
    parent = "mutations"
+++

The upsert block makes managing external IDs easy.

Set the schema.
```
xid: string @index(exact) .
<http://schema.org/name>: string @index(exact) .
<http://schema.org/type>: [uid] @reverse .
```

Set the type first of all.
```
{
  set {
    _:blank <xid> "http://schema.org/Person" .
    _:blank <dgraph.type> "ExternalType" .
  }
}
```

Now you can create a new person and attach its type using the upsert block.
```
   upsert {
      query {
        var(func: eq(xid, "http://schema.org/Person")) {
          Type as uid
        }
        var(func: eq(<http://schema.org/name>, "Robin Wright")) {
          Person as uid
        }
      }
      mutation {
          set {
           uid(Person) <xid> "https://www.themoviedb.org/person/32-robin-wright" .
           uid(Person) <http://schema.org/type> uid(Type) .
           uid(Person) <http://schema.org/name> "Robin Wright" .
           uid(Person) <dgraph.type> "Person" .
          }
      }
    }
```

You can also delete a person and detach the relation between Type and Person Node. It's the same as above, but you use the keyword "delete" instead of "set". "`http://schema.org/Person`" will remain but "`Robin Wright`" will be deleted.

```
   upsert {
      query {
        var(func: eq(xid, "http://schema.org/Person")) {
          Type as uid
        }
        var(func: eq(<http://schema.org/name>, "Robin Wright")) {
          Person as uid
        }
      }
      mutation {
          delete {
           uid(Person) <xid> "https://www.themoviedb.org/person/32-robin-wright" .
           uid(Person) <http://schema.org/type> uid(Type) .
           uid(Person) <http://schema.org/name> "Robin Wright" .
           uid(Person) <dgraph.type> "Person" .
          }
      }
    }
```

Query by user.
```
{
  q(func: eq(<http://schema.org/name>, "Robin Wright")) {
    uid
    xid
    <http://schema.org/name>
    <http://schema.org/type> {
      uid
      xid
    }
  }
}
```