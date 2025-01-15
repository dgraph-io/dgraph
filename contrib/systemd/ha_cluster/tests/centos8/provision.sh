#!/usr/bin/env bash

#####
# main
################################
main() {
	if [[ $1 =~ h(elp)?|\? ]]; then usage; fi
	if (($# != 1)); then usage; fi
	REPLICAS=$1

	echo "RUNNING script"

	setup_hosts
	install_dgraph
	setup_user_group
	setup_systemd
	setup_firewall
}

#####
# usage
################################
usage() {
	printf "   Usage: \n\t$0 [REPLICAS]\n\n" >&2
	exit 1
}

#####
# install_dgraph - installer script from https://get.dgraph.io
################################
install_dgraph() {
	[[ -z ${DGRAPH_VERSION} ]] && {
		echo 'DGRAPH_VERSION not specified. Aborting' 2>&1
		return 1
	}
	echo "INFO: Installing Dgraph with 'curl -sSf https://get.dgraph.io | ACCEPT_LICENSE="y" VERSION=""${DGRAPH_VERSION}"" bash'"
	curl -sSf https://get.dgraph.io | ACCEPT_LICENSE="y" VERSION="${DGRAPH_VERSION}" bash
}

#####
# setup_hosts - configure /etc/hosts in absence of DNS
################################
setup_hosts() {
	CONFIG_FILE=/vagrant/hosts
	if [[ ! -f /vagrant/hosts ]]; then
		echo "INFO: '${CONFIG_FILE}' does not exist. Skipping configuring /etc/hosts"
		return 1
	fi

	while read -a LINE; do
		## append to hosts entry if it doesn't exist
		if ! grep -q "${LINE[1]}" /etc/hosts; then
			printf "%s %s \n" "${LINE[*]}" >>/etc/hosts
		fi
	done <"${CONFIG_FILE}"
}

#####
# setup_user_group - dgraph user and gruop
################################
setup_user_group() {
	id -g dgraph &>/dev/null || groupadd --system dgraph
	id -u dgraph &>/dev/null || useradd --system -d /var/lib/dgraph -s /bin/false -g dgraph dgraph
}

#####
# setup_firewall on Ubuntu 18.04 and CentOS 8
################################
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
		if /usr/bin/firewall-cmd --state 2>&1 | grep -q "^running$"; then
			for PORT in ${PORTS[*]}; do
				firewall-cmd --zone=public --permanent --add-port="${PORT}"/tcp
				firewall-cmd --reload
			done
		fi
	elif grep -iq ubuntu /etc/os-release; then
		if /usr/sbin/ufw status | grep -wq active; then
			for PORT in ${PORTS[*]}; do
				ufw allow from any to any port "${PORT}" proto tcp
			done
		fi
	fi
}

#####
# setup_systemd_zero - setup dir and systemd unit for zero leader or peer
################################
setup_systemd_zero() {
	TYPE=${1:-"peer"}
	LDR="zero-0:5080"
	WAL=/var/lib/dgraph/zw
	IDX=$(($(grep -o '[0-9]' <<<"${HOSTNAME}") + 1))
	if [[ ${TYPE} == "leader" ]]; then
		EXEC="/bin/bash -c '/usr/local/bin/dgraph zero --my=\$(hostname):5080 --wal ${WAL}
    --raft="idx=${IDX}" --replicas ${REPLICAS}'"
	else
		EXEC="/bin/bash -c '/usr/local/bin/dgraph zero --my=\$(hostname):5080 --peer ${LDR} --wal ${WAL}
    --raft="idx=${IDX}" --replicas ${REPLICAS}'"
	fi

	mkdir -p /var/{log/dgraph,lib/dgraph/zw}
	chown -R dgraph:dgraph /var/{lib,log}/dgraph

	install_systemd_unit "zero" "${EXEC}"
}

#####
# setup_systemd_alpha - setup dir and systemd unit for alpha
################################
setup_systemd_alpha() {
	WAL=/var/lib/dgraph/w
	POSTINGS=/var/lib/dgraph/p
	# build array based on number of replicas
	for ((I = 0; I <= REPLICAS - 1; I++)); do ZEROS+=("zero-${I}:5080"); done
	IFS=, eval 'ZERO_LIST="${ZEROS[*]}"' # join by ','

	EXEC="/bin/bash -c '/usr/local/bin/dgraph alpha --my=\$(hostname):7080 --zero ${ZERO_LIST} --postings ${POSTINGS} --wal ${WAL}'"

	mkdir -p /var/{log/dgraph,lib/dgraph/{w,p}}
	chown -R dgraph:dgraph /var/{lib,log}/dgraph

	install_systemd_unit "alpha" "${EXEC}"
}

#####
# install_systemd_unit - config systemd unit give exec str and service type
################################
install_systemd_unit() {
	TYPE=$1
	EXEC=$2

	if [[ ! -f /etc/systemd/system/dgraph-${TYPE}.service ]]; then
		cat <<-EOF >/etc/systemd/system/dgraph-"${TYPE}".service
			[Unit]
			Description=dgraph ${TYPE} server
			Wants=network.target
			After=network.target

			[Service]
			Type=simple
			WorkingDirectory=/var/lib/dgraph
			Restart=on-failure
			ExecStart=${EXEC}
			StandardOutput=journal
			StandardError=journal
			User=dgraph
			Group=dgraph

			[Install]
			WantedBy=multi-user.target
		EOF
		systemctl enable dgraph-"${TYPE}"
		systemctl start dgraph-"${TYPE}"
	else
		echo "Skipping as 'dgraph-${TYPE}.service' already exists"
	fi
}

#####
# setup_systemd - configure systemd unit based on hostname
################################
setup_systemd() {
	case $(hostname) in
	*zero-0*)
		setup_systemd_zero "leader"
		;;
	*zero-[1-9]*)
		setup_systemd_zero "peer"
		;;
	*alpha*)
		setup_systemd_alpha
		;;
	esac
}

main $@
