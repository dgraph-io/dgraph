# Contribute to DGraph

**Pull requests are welcome.**

## Style Guide
We're following [Go Code Review](https://github.com/golang/go/wiki/CodeReviewComments).
Use `gofmt` for formatting your code. Avoid unnecessary vertical spaces.
Wrap your code and comments to 100 characters, unless doing so makes the code less legible.

## Code Review Workflow
- All contributors need to sign the [Contributor License Agreement](https://cla-assistant.io/dgraph-io/dgraph).
- Pick an issue from [pending issues](https://github.com/dgraph-io/dgraph/issues).
- If the issue isn't currently present in the list, file a new issue,
or bring it up on [Google Group](https://groups.google.com/forum/#!forum/dgraph) or [Gitter Chat](https://gitter.im/dgraph-io/dgraph),
so someone can file an issue on your behalf.
- Indicate that you're working on the issue by assigning the issue to yourself.
- Create a new branch, write your code, and commit your changes locally.
- Make sure you run `go test ./...` from the root.
- [Create a pull request](https://help.github.com/articles/creating-a-pull-request/)
- DGraph uses [Reviewable](https://reviewable.io/) for code reviews, and follows a rigorous code review process.
- Address the comments, and repeat the cycle, until you get an LGTM by someone qualified.
- Once you have an LGTM, go ahead and merge.
Most new contributors aren't allowed to merge themselves, in that case, we'll do it for you.
