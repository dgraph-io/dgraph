set -ex

exec dgraph zero --my=$(hostname -f):5080
