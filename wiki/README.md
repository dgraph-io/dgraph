# Dgraph Wiki

If you are looking for Dgraph documentation, you might find https://docs.dgraph.io much more readable.

## Contributing

We use [Hugo](https://gohugo.io/s) for our documentation.

### Running locally

1. Download and install hugo from [here](https://github.com/spf13/hugo/releases).
2. From within the `wiki` folder, run the command below to get the theme.

```
cd themes && git clone https://github.com/dgraph-io/hugo-docs
```

3. Run `./scripts/local.sh` from within the `wiki` folder and goto `http://localhost:1313` to see the Wiki.

We use `./scripts/local.sh` script to set env variables that our documentation theme internally uses.

Now you can make changes to the docs and see them being updated instantly thanks to Hugo.

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
