```shell
dgraph -dumpsg dumpsg -port 8912
```


```shell
curl localhost:8912/query -XPOST -d '{
        me(_xid_:m.08624h) {
         type.object.name.en
         film.actor.film {
             film.performance.film {
                 type.object.name.en
             }
         }
    }
}' | python -m json.tool

curl localhost:8912/query -XPOST -d '{
        me(_xid_:m.08624h) {
         type.object.name.en
         film.actor.film {
             film.performance.film {
                 type.object.name.en
             }
         }
    }
}' | python -m json.tool
```