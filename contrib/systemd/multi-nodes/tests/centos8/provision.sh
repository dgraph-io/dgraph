#!/usr/bin/env bash

main() {
  if [[ $1 =~ h(elp)?|\? ]]; then usage; fi
  if (( $# != 1 )); then usage; fi
  REPLICAS=$1

  install_dgraph
  setup_user_group
  setup_systemd
  setup_firewall
}

usage() {
  printf "   Usage: \n\t$0 [REPLICAS]\n\n" >&2
  exit 1
}

install_dgraph() {
  [[ -z "$DGRAPH_VERSION" ]] && { echo 'DGRAPH_VERSION not specified. Aborting' 2>&1 ; return 1; }
  curl -sSf https://get.dgraph.io | ACCEPT_LICENSE="y" VERSION="$DGRAPH_VERSION" bash
}

setup_user_group() {
	id -g dgraph &>/dev/null || groupadd --system dgraph
	id -u dgraph &>/dev/null || useradd --system -d /var/lib/dgraph -s /bin/false -g dgraph dgraph
}

setup_firewall() {
    case $(hostname) in
      *zero*)
        PORTS=(5080 6080)
        ;;
      *alpha*)
        PORTS=(7080 8080 9080)
        ;;
    esac

  if grep -q centos /etc/os-release; then
    for PORT in ${PORTS[*]}; do
      firewall-cmd --zone=public --permanent --add-port=$PORT/tcp
    done
  fi
}

setup_systemd_zero() {
  TYPE=${1:-"peer"}
  LDR="zero0:5080"
  WAL=/var/lib/dgraph/zw
  IDX=$(( $(grep -o '[0-9]' <<< $HOSTNAME) + 1 ))
  if [[ $TYPE == "leader" ]]; then
    EXEC="/usr/local/bin/dgraph zero --my=$(hostname):5080 --wal $WAL --idx=$IDX --replicas $REPLICAS"
  else
    EXEC="/usr/local/bin/dgraph zero --my=$(hostname):5080 --peer $LDR --wal $WAL --idx=$IDX --replicas $REPLICAS"
  fi

  mkdir -p /var/{log/dgraph,lib/dgraph/zw}
  chown -R dgraph:dgraph /var/{lib,log}/dgraph

  install_systemd_unit "zero" "$EXEC"
}

setup_systemd_alpha() {
  WAL=/var/lib/dgraph/w
  POSTINGS=/var/lib/dgraph/p
  # build array based on number of replicas
  for (( I=0; I <= $REPLICAS-1; I++)); do ZEROS+=("zero$I:5080");done
  IFS=, eval 'ZERO_LIST="${ZEROS[*]}"' # join by ','

  EXEC="/usr/bin/bash -c '/usr/local/bin/dgraph alpha --my=\$(hostname):7080 --lru_mb 2048 --zero $ZERO_LIST:5080 --postings $POSTINGS --wal $WAL'"

  mkdir -p /var/{log/dgraph,lib/dgraph/{w,p}}
  chown -R dgraph:dgraph /var/{lib,log}/dgraph

  install_systemd_unit "alpha" "$EXEC"
}

install_systemd_unit() {
  TYPE=$1
  EXEC=$2

  if [[ ! -f /etc/systemd/system/dgraph-$TYPE.service ]]; then
    cat <<-EOF > /etc/systemd/system/dgraph-$TYPE.service
[Unit]
Description=dgraph $TYPE server
Wants=network.target
After=network.target

[Service]
Type=simple
WorkingDirectory=/var/lib/dgraph
Restart=on-failure
ExecStart=$EXEC
StandardOutput=journal
StandardError=journal
User=dgraph
Group=dgraph

[Install]
WantedBy=multi-user.target
EOF
    systemctl enable dgraph-$TYPE
    systemctl start dgraph-$TYPE
  else
    echo "Skipping as 'dgraph-$TYPE.service' already exists"
  fi
}

setup_systemd() {
  case $(hostname) in
    *zero0*)
      setup_systemd_zero "leader"
      ;;
    *zero[1-9]*)
      setup_systemd_zero "peer"
      ;;
    *alpha*)
      setup_systemd_alpha
      ;;
  esac
}

main $@
