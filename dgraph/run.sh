#!/bin/bash
for o in $@; do
    case $o in
        '--jaeger' )
            EXTRA_COMPOSE=( -f docker-compose-jaeger.yml )
            ;;
        *)
            echo "Unknown option $1"
            exit 1
            ;;
    esac
    shift
done
make install
docker-compose down
DATA=$HOME/dg docker-compose -f docker-compose.yml ${EXTRA_COMPOSE[@]} up --force-recreate --remove-orphans
