# Docker build script

This directory contains a Makefile that can be used to build Dgraph inside the official Dgraph
Docker container. This is useful for situations when the host system cannot be used to build a
binary that will work with the container (for example, if the host system has a different version of
glibc).

## Usage

Run `make install` in this directory. The script will ask you for your password in order to change
ownership of the compiled binary. By default, files written by Docker will be owned by root. This
script also takes care of moving the binary to $GOPATH/bin.
