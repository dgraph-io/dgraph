# Dgraph Release Process

This document outlines the steps needed to build and push a new release of Dgraph

1. Have a team member "at-the-ready" with github `writer` access (you'll need them to approve PRs)
1. Create a new branch (prepare-for-release-vXX.X.X, for instance).
1. Update dependencies in `go.mod` for Badger, Ristretto and Dgo, if required.
1. If you didn't update `go.mod`, decide if you need to run the weekly upgrade CI workflow, [ci-dgraph-weekly-upgrade-tests](https://github.com/dgraph-io/dgraph/actions/workflows/ci-dgraph-weekly-upgrade-tests.yml), it takes about 60-70 minutes… Note that you only need to do this if there are changes on the main branch after the last run (it runs weekly on Sunday nights).
1. Update the CHANGELOG.md. Sonnet 4.5 does a great job of doing this. Example prompt: `I'm releasing vXX.X.X off the main branch, add a new entry for this release. Conform to the "Keep a Changelog" format, use past entries as a formatting guide. Run the trunk linter on your changes.`. Commit and push your changes. Create a PR and have a team member approve it. If you've only modified markdown content, the code-based CI steps will be skipped.
1. On the github releases page, create a new draft release (this will create the tag, in this example we're releasing _v25.1.0-preview1_):
    <img width="1044" height="775" alt="image" src="https://gist.github.com/user-attachments/assets/cb6fefed-d875-4a98-bd44-26b62cb4e812" />
    Note: check the toggle to create a Annoucement in our Discussions forum (time saver).
1. Start the deployment workflow from [here](https://github.com/dgraph-io/dgraph/actions/workflows/cd-dgraph.yml):
    <img width="1491" height="745" alt="image" src="https://gist.github.com/user-attachments/assets/bda5b8b1-d9ad-4931-b031-df6c6a4f919a" />
    Note: only select the "if checked, images will be pushed to dgraph-custom" if this release is an unreleased patch.

    The CD workflow handles the building, tagging and copying of release artifacts to dockerhub and the releases area.
1. Verify the push
    1. Ensure the image tags are pushed to [https://hub.docker.com/r/dgraph/dgraph/tags](https://hub.docker.com/r/dgraph/dgraph/tags)
    1. Run this command to verify the version: `docker run dgraph/dgraph:vXX.X.X dgraph version`
    1. The new CD workflow _should_ copy all artifacts to the release (there should be 10 artifacts with the release). If not, at the end of the CD workflow, you'll find the created artifacts that you'll need to download and then re-upload to the release, sigh... (click on the 'edit' button of the release--use artifact names from prior release as a guide)

