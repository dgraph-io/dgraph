# Git hooks

* The pre-push hook runs tests before pushing.

* The pre-commit hook checks for go vet and golint errors for the staged files whose content was changed.

* I took inspiration for golint.sh and govet.sh from https://github.com/youtube/vitess/tree/master/misc/git/hooks.

## The files in this folder can be symlinked to those in .git/hooks using the following commands.

```
# from the root of the repo, move into the git folder.
$ cd .git
# create symlink between directories
$ ln -s ../contrib/hooks hooks
```

Now everytime you do a `git push`, tests should be run for you.
And before a commit, you'd have the option to see results from and golint.
Also, if go vet shows any errors you won't be allowed to commit without correcting them.
