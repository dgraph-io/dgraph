#!/usr/bin/env bash
# shellcheck disable=SC2310,SC2329
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

find_protoc() {
	command -v protoc 2>/dev/null
}

install_protoc_linux() {
	# apt/dnf/yum use protobuf-compiler, pacman uses protobuf
	install_linux_pkg "protobuf-compiler" "protobuf-compiler" "protobuf-compiler" "protobuf"
}

install_protoc_macos() {
	ensure_brew || exit 1
	brew install protobuf
}

install_protoc() {
	run_os_installer install_protoc_linux install_protoc_macos
}

main() {
	if find_protoc &>/dev/null; then
		exit 0
	fi

	if [[ ${AUTO_INSTALL-} == "true" ]]; then
		install_protoc
		if find_protoc &>/dev/null; then
			exit 0
		else
			err "protoc check still failing after installation"
			exit 1
		fi
	fi

	echo ""
	err "protoc is not installed"
	echo ""
	err "Please install protoc manually:"

	case "$(get_os)" in
	Linux)
		err "    apt:    sudo apt-get install protobuf-compiler"
		err "    dnf:    sudo dnf install protobuf-compiler"
		err "    yum:    sudo yum install protobuf-compiler"
		err "    pacman: sudo pacman -S protobuf"
		;;
	Darwin)
		err "    brew install protobuf"
		;;
	*)
		err "    See: https://grpc.io/docs/protoc-installation/"
		;;
	esac

	print_auto_install_hint
	exit 1
}

main "$@"
