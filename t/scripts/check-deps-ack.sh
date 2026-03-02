#!/usr/bin/env bash
# shellcheck disable=SC2310,SC2329
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

# Find ack binary (may be 'ack' or 'ack-grep' on older Debian/Ubuntu)
find_ack() {
	command -v ack 2>/dev/null || command -v ack-grep 2>/dev/null
}

install_ack_linux() {
	if command -v apt-get &>/dev/null; then
		sudo apt-get update
		# Try 'ack' first (newer distros), fall back to 'ack-grep' (older distros)
		if apt-cache show ack &>/dev/null; then
			sudo apt-get install -y ack
		else
			sudo apt-get install -y ack-grep
		fi
	else
		# dnf/yum/pacman all use 'ack'
		install_linux_pkg "ack"
	fi
}

install_ack_macos() {
	ensure_brew || exit 1
	brew install ack
}

install_ack() {
	run_os_installer install_ack_linux install_ack_macos
}

main() {
	if find_ack &>/dev/null; then
		exit 0
	fi

	if [[ ${AUTO_INSTALL-} == "true" ]]; then
		install_ack
		if find_ack &>/dev/null; then
			exit 0
		else
			err "ack check still failing after installation"
			exit 1
		fi
	fi

	echo ""
	err "ack is not installed"
	echo ""
	err "Please install ack manually:"

	case "$(get_os)" in
	Linux)
		err "    apt:    sudo apt-get install ack  (or ack-grep on older systems)"
		err "    dnf:    sudo dnf install ack"
		err "    yum:    sudo yum install ack"
		err "    pacman: sudo pacman -S ack"
		;;
	Darwin)
		err "    brew install ack"
		;;
	*)
		err "    (see https://beyondgrep.com/install/)"
		;;
	esac

	print_auto_install_hint
	exit 1
}

main "$@"
