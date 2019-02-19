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
    --one-group    Run a single replicated Alpha group (3 Zeros, 3 Alphas).
    --metrics      Run with metrics collection and dashboard (Prometheus/Grafana/Node Exporter).
    --data <path>  Run with Dgraph data directories mounted to <path>.
    --tmpfs        Run with WAL directories (w/zw) in-memory with tmpfs.

Metrics are stored in separate Docker volumes for persistence across runs.
To reset metrics collection, remove any existing volumes by running:

    docker volume rm dgraph_prometheus-volume
    docker volume rm dgraph_grafana-volume
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
            SINGLE_FLAG=""
            if [ ! -z "${ONE_GROUP_FLAG+x}" ]; then
                echo '--single and --one-group not allowed at the same time'
                exit 1
            fi
            ;;
        '--one-group' )
            SERVICES+=( zero1 zero2 zero3 dg1 dg2 dg3 )
            ONE_GROUP_FLAG=""
            if [ ! -z "${SINGLE_FLAG+x}" ]; then
                echo '--single and --one-group not allowed at the same time'
                exit 1
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

# If the --single or --one-group flag was set, then make sure that services from
# other flags are started too.
if [ "${#SERVICES[@]}" -gt 0 ]; then
    if [[ " ${COMPOSE_FILES[@]} " =~ "docker-compose-jaeger.yml" ]]; then
        SERVICES+=( jaeger )
    fi
    if [[ " ${COMPOSE_FILES[@]} " =~ "docker-compose-metrics.yml" ]]; then
        SERVICES+=( node-exporter prometheus grafana )
    fi
fi

make install
docker-compose ${COMPOSE_FILES[@]} down
DATA=$DATA docker-compose ${COMPOSE_FILES[@]} up --force-recreate --remove-orphans ${SERVICES[@]}
