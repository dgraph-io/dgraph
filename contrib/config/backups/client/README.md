# Backup Script

This backup script that supports many of the features in Dgraph, such as ACLs, MutualTLS, REST or GraphQL API.  See `./backup.sh --help` for all of the options.

## Requirements

This script requires GNU getopt for command line parameters. This was tested on macOS with Homebrew [gnu-getopt](https://formulae.brew.sh/formula/gnu-getopt) bottle, Ubuntu 20.04, and Windows with [MSYS2](https://www.msys2.org/).

## Important Notes

If you are using this script another system other than alpha, we'll call this *backup worksation*, you should be aware of the following:

* General
  * backup workstation will need access to alpha server
* TLS
  * when accessing alpha server secured by TLS, backup workstation will need access to `ca.crt` created with `dgraph cert`
  * if MutualTLS is used, backup worksation will also need access to the client cert and key as well.
* subpath option
  * when specifying subpath that uses a datestamp, the backup workstation needs to have the same timestamp as the server.
  * when backing to a filepath, such as NFS, the backup workstation will need access to the samefile path.
