#!/bin/bash
COMPOSE_FILES=( -f docker-compose.yml )
SERVICES=()
for o in $@; do
    case $o in
        '-h'|'--help' )
            cat <<EOF
Runs the Dgraph test cluster.

Available options:

    --jaeger   Run with Jaeger tracing enabled.
    --single   Run a single Zero and Alpha (2-node cluster).
    --metrics  Run with metrics collection and dashboard (Prometheus/Grafana/Node Exporter).
EOF
            exit 0
            ;;
        '--jaeger' )
            COMPOSE_FILES+=( -f docker-compose-jaeger.yml )
            ;;
        '--metrics' )
            COMPOSE_FILES+=( -f docker-compose-metrics.yml )
            ;;
        '--single' )
            SERVICES=( zero1 dg1 )
            if [[ " ${COMPOSE_FILES[@]} " =~ "docker-compose-jaeger.yml" ]]; then
                SERVICES+=( jaeger )
            fi
            if [[ " ${COMPOSE_FILES[@]} " =~ "docker-compose-metrics.yml" ]]; then
                SERVICES+=( node-exporter prometheus grafana )
            fi
            ;;
        *)
            echo "Unknown option $1"
            exit 1
            ;;
    esac
    shift
done

make install
docker-compose ${COMPOSE_FILES[@]} down
export DATA=/mnt/nvme/dgraph
docker-compose ${COMPOSE_FILES[@]} up --force-recreate --remove-orphans ${SERVICES[@]}

