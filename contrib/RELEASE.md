# Dgraph Release Process

This document outlines the steps needed to build and push a new release of Dgraph.

1. Have a team member "at-the-ready" with github `writer` access (you'll need them to approve PRs).
1. Create a new branch (prepare-for-release-vXX.X.X, for instance).
1. Update dependencies in `go.mod` for Badger, Ristretto and Dgo, if required.
1. If you didn't update `go.mod`, decide if you need to run the weekly upgrade CI workflow,
   [ci-dgraph-weekly-upgrade-tests](https://github.com/dgraph-io/dgraph/actions/workflows/ci-dgraph-weekly-upgrade-tests.yml),
   it takes about 60-70 minutes… Note that you only need to do this if there are changes on the main
   branch after the last run (it runs weekly on Sunday nights).
1. Update the CHANGELOG.md. Sonnet 4.5 does a great job of doing this. Example prompt:
   `I'm releasing vXX.X.X off the main branch, add a new entry for this release. Conform to the "Keep a Changelog" format, use past entries as a formatting guide. Run the trunk linter on your changes.`.
1. Validate the version does not have storage incompatibilities with the previous version. If so, add a warning to the CHANGELOG.md
   that export/import of data will need to be run as part of the upgrade process.
1. Commit and push your changes. Create a PR and have a team member approve it. If you've only
   modified markdown content, the code-based CI steps will be skipped.
1. Using the updated CHANGELOG.md as a guide, ensure that user-facing additions or changes have appropriate PRs in
   the Dgraph [docs repo](https://github.com/dgraph-io/dgraph-docs/).
1. Once your "prepare for release branch" is merged into main, on the github
   [releases](https://github.com/dgraph-io/dgraph/releases) page, create a new draft release (this step
   will create the tag, in this example we're releasing _v25.1.0-preview1_):
   <img width="1491" height="745" alt="image" src="https://github.com/user-attachments/assets/18a2af83-9345-48fc-95cf-368ea4f0a70e" />
   Note: check the toggle to create a Annoucement in our Discussions forum (time saver).
1. Start the deployment workflow from
   [here](https://github.com/dgraph-io/dgraph/actions/workflows/cd-dgraph.yml):
   <img width="1491" height="745" alt="image" src="https://github.com/user-attachments/assets/9f3801d1-56dd-4bfe-acdb-9f1518b0bf13" />
   Note: only select the "if checked, images will be pushed to dgraph-custom" if this release is an
   unreleased patch.

   The CD workflow handles the building, tagging and copying of release artifacts to dockerhub and
   the releases area.

1. Verify the push
   1. Ensure the image tags are pushed to
      [https://hub.docker.com/r/dgraph/dgraph/tags](https://hub.docker.com/r/dgraph/dgraph/tags)
   1. Run this command to verify the version: `docker run dgraph/dgraph:vXX.X.X dgraph version`
   1. The new CD workflow _should_ copy all artifacts to the release (there should be 10 artifacts
      with the release). If not, at the end of the CD workflow, you'll find the created artifacts
      that you'll need to download and then re-upload to the release, sigh... (click on the 'edit'
      button of the release--use artifact names from prior release as a guide)
1. For all releases (non-previews and non-patches), create a release branch. In order to easily
   backport fixes to the release branch, create a release branch from the tag head. For instance, if
   we're releasing v25.1.0, create a branch called `release/v25.1` from the tag head (ensure you're
   on the main branch from which you created the tag)
   ```sh
   git checkout main
   git pull origin main
   git checkout -b release/v25.1
   git push origin release/v25.1
   ```
1. Check the [Discussions](https://github.com/orgs/dgraph-io/discussions) forum for the announcement
   of the release (create one if you forgot that step above which autocreates one).
1. If the release has documentation changes, alert the docs team to merge the doc changes in the 
   [dgraph-docs](https://github.com/dgraph-io/dgraph-docs/) repo.
