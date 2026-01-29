#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

find_jq() {
	command -v jq 2>/dev/null
}

install_jq_linux() {
	install_linux_pkg "jq"
}

install_jq_macos() {
	ensure_brew || exit 1
	brew install jq
}

install_jq() {
	run_os_installer install_jq_linux install_jq_macos
}

main() {
	if find_jq &>/dev/null; then
		exit 0
	fi

	if [[ ${AUTO_INSTALL-} == "true" ]]; then
		install_jq
		if find_jq &>/dev/null; then
			exit 0
		else
			err "jq check still failing after installation"
			exit 1
		fi
	fi

	echo ""
	err "jq is not installed"
	echo ""
	err "Please install jq manually:"

	case "$(get_os)" in
	Linux)
		err "    apt:    sudo apt-get install jq"
		err "    dnf:    sudo dnf install jq"
		err "    yum:    sudo yum install jq"
		err "    pacman: sudo pacman -S jq"
		;;
	Darwin)
		err "    brew install jq"
		;;
	*)
		err "    (see https://jqlang.github.io/jq/download/)"
		;;
	esac

	print_auto_install_hint
	exit 1
}

main "$@"
