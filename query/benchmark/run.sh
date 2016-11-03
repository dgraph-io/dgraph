set -e

# Where you store posting list and other data. It's where you start dgraph in.
DATADIR=$HOME/dgraph
THISDIR=`pwd`

ACTORS="m.03c7p9t m.0148x0 m.08624h"
DIRECTORS="m.0bysn41 m.03k5gd m.05dxl_"

pushd $DATADIR &> /dev/null

rm -Rf dumpsg

dgraph -dumpsg dumpsg -port 8912 &

sleep 2

for ACTOR in $ACTORS; do
  curl localhost:8912/query -XPOST -d "
  {
    me(_xid_:${ACTOR}) {
      type.object.name.en
      film.actor.film {
        film.performance.film {
          type.object.name.en
        }
      }
    }
  }" 2>/dev/null >/dev/null
done

n=0
for S in dumpsg/*.gob; do
  echo $S
  cp -vf $S $THISDIR/actor.${n}.gob
  n=$(($n+1))
done

rm -f dumpsg/*

for DIRECTOR in $DIRECTORS; do
  curl localhost:8912/query -XPOST -d "
  {
    me(_xid_:${DIRECTOR}) {
      type.object.name.en
      film.director.film {
        film.film.genre {
          type.object.name.en
        }
      }
    }
  }" 2>/dev/null >/dev/null
done

n=0
for S in dumpsg/*.gob; do
  echo $S
  cp -vf $S $THISDIR/director.${n}.gob
  n=$(($n+1))
done

rm -Rf dumpsg

killall dgraph

popd &> /dev/null
