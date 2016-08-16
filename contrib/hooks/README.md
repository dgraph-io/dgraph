# Git hooks

The pre-push hook runs tests before pushing.

The pre-commit hook checks for go vet and golint errors for the staged files whose content was changed.

Took inspiration for the golint.sh and govet.sh from https://github.com/youtube/vitess/tree/master/misc/git/hooks.

The files in this folder can be symlinked to those in .git/hooks.
