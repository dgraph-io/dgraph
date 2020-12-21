+++
date = "2017-03-20T22:25:17+11:00"
title = "Log Format"
weight = 9
[menu.main]
    parent = "deploy"
+++

Dgraph's log format comes from the glog library and is [formatted](https://github.com/golang/glog/blob/23def4e6c14b4da8ac2ed8007337bc5eb5007998/glog.go#L523-L533) as follows:

```
Lmmdd hh:mm:ss.uuuuuu threadid file:line] msg...
```

Where the fields are defined as follows:

```
	L                A single character, representing the log level (eg 'I' for INFO)
	mm               The month (zero padded; ie May is '05')
	dd               The day (zero padded)
	hh:mm:ss.uuuuuu  Time in hours, minutes and fractional seconds
	threadid         The space-padded thread ID as returned by GetTID()
	file             The file name
	line             The line number
	msg              The user-supplied message
```

## Increase Logging

To increase logging, you have set `-v=3` which will enable verbose logging for everything. You can set this flag on both Zero and Alpha nodes.

Alternatively, you can set `--vmodule=server=3` for only the Alpha node which would only enable query/mutation logging.

{{% notice "note" %}}
This requires a restart of the node itself.

Avoid keeping logging verbose for a long time. Revert back to default setting if not needed.
{{% /notice %}}

When enabling query logging with the `--vmodule=server=3` flag this prints the queries/mutation that Dgraph Cluster receives from Ratel, Client etc. In this case Alpha log will print something similar to:

```
I1201 13:06:26.686466   10905 server.go:908] Got a query: query:"{\n  query(func: allofterms(name@en, \"Marc Caro\")) {\n  uid\n  name@en\n  director.film\n  }\n}"  
```
As you can see, we got the query that Alpha received, to read it in the original DQL format just replace every `\n` with a new line, any `\t` with a tab character and `\"` with `"`:

```
{
  query(func: allofterms(name@en, "Marc Caro")) {
  uid
  name@en
  director.film
  }
}
```

You can also dynamically turn query logging on or off without restarting any Alpha node. To toggle query logging on or off, send the following GraphQL mutation to the `/admin` endpoint of an Alpha node (e.g. `localhost:8080/admin`):

```graphql
mutation {
  config(input: {logRequest: true}){
    response {
      code
      message
    }
  }
}
```
The response should look like the following:

```json
{
  "data": {
    "config": {
      "response": {
        "code": "Success",
        "message": "Config updated successfully"
      }
    }
  },
  "extensions": {
    "tracing": {
      "version": 1,
      "startTime": "2020-12-07T14:53:28.240420495Z",
      "endTime": "2020-12-07T14:53:28.240569604Z",
      "duration": 149114
    }
  }
}
```
Also, the Alpha node will print the following INFO message to confirm that the mutation has been applied:
```
I1207 14:53:28.240516   20143 config.go:39] Got config update through GraphQL admin API
```

Similarly, you can turn off query logging by setting `logRequest` to `false` in the above mutation.
