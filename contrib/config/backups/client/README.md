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

## Testing (Demo)

You can try out these features using [Docker Compose](https://docs.docker.com/compose/).  There's a `./setup.sh` script that can configure the environment with the desired features.  As you need to have a common shared directory for filepaths, you can use `ratel` container to run the backup script.

As an example:

```bash
## configure docker-compose environment
./setup.sh --acl --enc --tls --make_tls_cert
## run demo
docker-compose up -d
## login into Ratel to use for backups
docker exec --tty --interactive ratel bash
```

Then in the Ratel container, run:

```bash
## trigger a backup on alpha1
./backup.sh \
  --alpha alpha1 \
  --tls_cacert /dgraph/tls/ca.crt \
  --force_full \
  --location /dgraph/backups \
  --user groot \
  --password password

## check for backup files
ls /dgraph/backups
```
