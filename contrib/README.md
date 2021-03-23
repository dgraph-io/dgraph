# Contributor Resources

The `contrib` directory contains scripts, images, and other helpful things
which are not part of the core dgraph distribution. Please note that they
could be out of date, since they do not receive the same attention as the
rest of the repository.

## Building the Docker Image

The docker image is meant to be built from the root directory of the project and can be built with the following command:

    docker build -t dgraph/dgraph:latest --file contrib/Dockerfile .
