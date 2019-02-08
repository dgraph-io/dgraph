#!/bin/bash
COMPOSE_FILES=( -f docker-compose.yml )
SERVICES=()

while (( "$#" )); do
    case "$1" in
        '-h'|'--help' )
            cat <<EOF
Runs the Dgraph test cluster.

Available options:

    --jaeger       Run with Jaeger tracing enabled.
    --single       Run a single Zero and Alpha (2-node cluster).
    --metrics      Run with metrics collection and dashboard (Prometheus/Grafana/Node Exporter).
    --data <path>  Run with Dgraph data directories mounted to <path>.
    --tmpfs        Run with WAL directories (w/zw) in-memory with tmpfs.
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
            SERVICES+=( zero1 dg1 )
            if [[ " ${COMPOSE_FILES[@]} " =~ "docker-compose-jaeger.yml" ]]; then
                SERVICES+=( jaeger )
            fi
            if [[ " ${COMPOSE_FILES[@]} " =~ "docker-compose-metrics.yml" ]]; then
                SERVICES+=( node-exporter prometheus grafana )
            fi
            ;;
        '--data' )
            DATA="$2"
            shift
            if [ -z "${DATA}" ]; then
                echo "--data option missing <path>"
                exit 1
            fi
            if [ ! -d "$DATA" ]; then
                echo "Directory does not exist: $DATA"
            fi
            COMPOSE_FILES+=( -f docker-compose-data.yml )
            echo "Running with Dgraph data directories mounted at $DATA"
            ;;
        '--tmpfs' )
            COMPOSE_FILES+=( -f docker-compose-tmpfs.yml )
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
DATA=$DATA docker-compose ${COMPOSE_FILES[@]} up --force-recreate --remove-orphans ${SERVICES[@]}
