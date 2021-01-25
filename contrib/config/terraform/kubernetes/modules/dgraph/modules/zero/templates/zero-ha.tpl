set -ex

[[ `hostname` =~ -([0-9]+)$ ]] || exit 1

ordinal=$${BASH_REMATCH[1]}
idx=$(($ordinal + 1))

if [[ $ordinal -eq 0 ]]; then
  exec dgraph zero --my=$(hostname -f):5080 --raft="idx=$idx" --replicas ${replicas}
else
  exec dgraph zero --my=$(hostname -f):5080 --peer
  ${prefix}-dgraph-zero-0.${prefix}-dgraph-zero.${namespace}.svc.cluster.local:5080
  --raft="idx=$idx" --replicas ${replicas}
fi
