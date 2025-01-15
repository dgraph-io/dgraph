#!/usr/bin/env bash
sudo_cmd=""
if hash sudo 2>/dev/null; then
	sudo_cmd="sudo"
	echo "Requires sudo permission to install Dgraph in Systemd."
	if ! ${sudo_cmd} -v; then
		print_error "Need sudo privileges to complete installation."
		exit 1
	fi
fi

${sudo_cmd} groupadd --system dgraph
${sudo_cmd} useradd --system -d /var/lib/dgraph -s /bin/false -g dgraph dgraph
${sudo_cmd} mkdir -p /var/log/dgraph
${sudo_cmd} mkdir -p /var/lib/dgraph/{p,w,zw}
${sudo_cmd} chown -R dgraph:dgraph /var/{lib,log}/dgraph
