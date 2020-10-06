# Dgraph Wiki

If you are looking for Dgraph documentation, you might find https://dgraph.io/docs/ much more readable.

## Contributing

We use [Hugo](https://gohugo.io/) for our documentation.

### Running locally

1. Download and install the latest patch of hugo version v0.69.x from [here](https://github.com/gohugoio/hugo/releases/).

2. From within the `wiki` folder, run the command below to get the theme.

```
pushd themes && git clone https://github.com/dgraph-io/hugo-docs && popd
```

3. Run `./scripts/local.sh` within the `wiki` folder and visit `http://localhost:1313` to see the
documentation site.

(Optional) To run queries *within* the documentation using a different Dgraph instance, set the `DGRAPH_ENDPOINT` environment variable before starting the local web server:
```
DGRAPH_ENDPOINT="http://localhost:8080/query?latency=true" ./scripts/local.sh
```

Now you can make changes to the docs and see them being updated instantly thanks to Hugo.

* While running locally, the version selector does not work because you need to build the documentation and serve it behind a reverse proxy to have multiple versions.

### Branch

Depending on what branch you are on, some code examples will dynamically change. For instance, go-grpc code examples will have different import path depending on branch name.


## Runnable

### Custom example

Pass custom Go-GRPC example to the runnable by passing a `customExampleGoGRPC` to the `runnable` shortcode.

```
{{< runnable
  customExampleGoGRPC="this\nis\nan example"
>}}{
  director(func:allofterms(name, "steven spielberg")) {
    name@en
    director.film (orderdesc: initial_release_date) {
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}
```

We cannot pass multiline string as an argument to a shortcode. Therefore, we
have to make the whole custom example in a single line string by replacing newlines with `\n`.

### Deployment

Run `./scripts/build.sh` in a tmux window. The script polls `dgraph-io/dgraph` every one minute
and pulls any new changes that have been merged to any of the branches listed in the script.
It also rebuilds the site if there are any changes.

Any new version for which docs need to be added should be added to the `VERSIONS_ARRAY` in
`scripts/build.sh` and the script should be restarted after SSHing into the server.

If for reason the site is not getting updated after pushing to the main repo, the script might have been
terminated. SSH into the server and restart it.
