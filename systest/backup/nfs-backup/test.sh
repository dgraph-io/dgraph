#!/bin/sh
docker ps -a
c=`docker ps | grep nfs_1 | cut -f1,2 -d " "`
docker logs $c
d=`docker ps | grep alpha7 | cut -f1,2 -d " "`
docker logs $d
