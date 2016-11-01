```shell
dgraph -dumpsg dumpsg -port 8912
```

# Actors query

We increase the number of actors until we hit just a little below the max.

```shell
rm -Rf dumpsg

NUMS="10 56 300"

for NUM in $NUMS; do
	curl localhost:8912/query -XPOST -d "{
	        me(_xid_:m.08624h) {
	         type.object.name.en
	         film.actor.film(first: $NUM) {
	             film.performance.film {
	                 type.object.name.en
	             }
	         }
	    }
	}" 2>/dev/null | python -m json.tool | wc -l
done

n=0
for S in dumpsg/*.gob; do
  echo $S
	cp -f $S /tmp/actor.${n}.gob
	n=$(($n+1))
done
```

# Directors query

We increase the number of directors until we hit just a little below the max.

```shell
rm -Rf dumpsg

NUMS="10 31 100"

for NUM in $NUMS; do
	curl localhost:8912/query -XPOST -d "{
        me(_xid_:m.05dxl_) {
          type.object.name.en
          film.director.film(first: $NUM)  {
                film.film.genre {
                  type.object.name.en
                }
          }
    }
	}" 2>/dev/null | python -m json.tool | wc -l
done

n=0
for S in dumpsg/*.gob; do
  echo $S
	cp -f $S /tmp/director.${n}.gob
	n=$(($n+1))
done
```

```
cp -vf /tmp/*.gob ./
```