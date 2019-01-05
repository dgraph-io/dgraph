#!/bin/bash
# Script to start/stop dgraph cluster defined in docker-compose.yml(s).
#
# vim: ts=4 sw=4 sts=4 et

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph

#==============================================================================
# FUNCTIONS
#==============================================================================

function Info
{
    echo -e "INFO: $*"
}

function DockerCompose
{
    docker-compose ${COMPOSE_FILES[@]} "$@" || exit $?
}

function ClusterDown
{
    Info "Bringing down cluster ..."
    DockerCompose --log-level=CRITICAL down
}

function ClusterUp
{
    Info "Bringing up cluster ..."
    DockerCompose up --force-recreate --remove-orphans --detach
    if [[ $WAIT ]]; then
        WaitTilReady
    fi
}

function RebuildDgraph
{
    Info "Rebuilding dgraph ..."
    pushd $DGRAPH_ROOT/dgraph >/dev/null
    make install || exit $?
    popd >/dev/null
}

function WaitTilReady
{
    Info "Wating for alphas to be ready ..."
    NUM_ALPHAS=$(DockerCompose ps | grep -c alpha)
    NUM_READY=0
    TIMEOUT=60
    while [[ $NUM_READY -lt $NUM_ALPHAS ]]; do
        sleep 1
        NUM_READY=$(DockerCompose logs \
                    | grep -c 'Got Zero leader')
        TIMEOUT=$((TIMEOUT - 1))
        if [[ $TIMEOUT == 0 ]]; then
            echo >&2 "$ME: timed out waiting for alphas"
            exit 1
        fi
    done
}

function FollowLogs
{
    while true; do
        DockerCompose logs -f || exit 1
        TIMEOUT=10
        while [[ $TIMEOUT -gt 0 ]]; do
            NUM_DGRAPH=$(DockerCompose ps | grep -c dgraph)
            if [[ $NUM_DGRAPH -gt 0 ]]; then
                echo -e "\n[CLUSTER RESTARTED]\n"
                sleep 1
                break
            fi
            TIMEOUT=$((TIMEOUT - 1))
        done
    done
}

function AssertClusterUp
{
    if [[ $(DockerCompose ps | grep -c dgraph) -eq 0 ]]; then
        echo >&2 "$ME: cluster not running"
        exit 1
    fi
}

#==============================================================================
# MAIN
#==============================================================================

unset HELP REBUILD WAIT

ARGS=$(/usr/bin/getopt -n$ME -o"hbw" -l"help,rebuild,wait" -- "$@") || exit 1
eval set -- "$ARGS"
while true; do
    case "$1" in
        -h|--help)      HELP=yes        ;;
        -b|--rebuild)   REBUILD=yes     ;;
        -w|--wait)      WAIT=yes        ;;
        --)             shift; break    ;;
    esac
    shift
done

if [[ $HELP ]]; then
    echo "usage: cluster [start|restart|stop|check|logs] [addtl_conf ...]"
    exit 0
fi

# if there are no more arguments, default to 'check' command
if [[ $# -eq 0 ]]; then
    CMD=check
else
    CMD=${1,,}  # lowercase $1
    shift
fi

# always use the top-level docker compose config
COMPOSE_FILES=("-f" "$DGRAPH_ROOT/dgraph/docker-compose.yml")

# if there are no more args look for additional config in the current dir,
# otherwise the args are configs or directories containing them
if [[ $# -eq 0 ]]; then
    ADDTL_CONF="./docker-compose.yml"
    if [[ $PWD != $DGRAPH_ROOT && -e ./docker-compose.yml ]]; then
        Info "Using additional config $ADDTL_CONF"
        COMPOSE_FILES+=("-f" "$ADDTL_CONF")
    fi
else
    while [[ $1 ]]; do
        if [[ -d $1 ]]; then
            ADDTL_CONF="$1/docker-compose.yml"
        elif [[ -e $1 ]]; then
            ADDTL_CONF="$1"
        else
            echo >&2 "$ME: no such file or directory -- $1"
            exit 1
        fi
        Info "Using additional config $ADDTL_CONF"
        COMPOSE_FILES+=("-f" "$ADDTL_CONF")
        shift
    done
fi

# finally, execute the command
case "$CMD" in
    check|ps)
        AssertClusterUp
        DockerCompose ps
        ;;
    logs)
        AssertClusterUp
        FollowLogs
        ;;
    start|up)
        if [[ $REBUILD ]]; then
            RebuildDgraph
        fi
        ClusterUp
        ;;
    restart)
        if [[ $REBUILD ]]; then
            RebuildDgraph
        fi
        ClusterDown
        ClusterUp
        ;;
    stop|down)
        AssertClusterUp
        ClusterDown
        ;;
    *)
        echo >&2 "$ME: invalid command -- $CMD"
        exit 1
esac

exit 0
