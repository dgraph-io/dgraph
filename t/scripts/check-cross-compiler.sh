#!/usr/bin/env bash
# shellcheck disable=SC2310
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

BREW_TAP="messense/macos-cross-toolchains"

# Return the expected cross-compiler binary name for the host architecture.
get_cross_compiler() {
	if [[ -n ${LINUX_CC-} ]]; then
		echo "${LINUX_CC}"
		return
	fi
	local arch
	arch="$(uname -m)"
	case "${arch}" in
	arm64 | aarch64)
		echo "aarch64-unknown-linux-gnu-gcc"
		;;
	x86_64)
		echo "x86_64-unknown-linux-gnu-gcc"
		;;
	*)
		err "unsupported architecture: ${arch}"
		return 1
		;;
	esac
}

# Return the Homebrew formula name for the cross-compiler.
get_brew_formula() {
	local arch
	arch="$(uname -m)"
	case "${arch}" in
	arm64 | aarch64)
		echo "aarch64-unknown-linux-gnu"
		;;
	x86_64)
		echo "x86_64-unknown-linux-gnu"
		;;
	*)
		err "unsupported architecture: ${arch}"
		return 1
		;;
	esac
}

install_cross_compiler_macos() {
	ensure_brew || exit 1
	local formula
	formula="$(get_brew_formula)"
	brew tap "${BREW_TAP}"
	brew install "${BREW_TAP}/${formula}"
}

main() {
	# Cross-compiler is only needed on non-Linux systems
	local os
	os="$(get_os)"
	if [[ ${os} == "Linux" ]]; then
		exit 0
	fi

	local cc
	cc="$(get_cross_compiler)"

	if command -v "${cc}" &>/dev/null; then
		exit 0
	fi

	if [[ ${AUTO_INSTALL-} == "true" ]]; then
		install_cross_compiler_macos
		if command -v "${cc}" &>/dev/null; then
			exit 0
		else
			err "cross-compiler check still failing after installation"
			exit 1
		fi
	fi

	echo ""
	err "Linux cross-compiler is not installed (needed for Go plugin builds)"
	echo ""
	err "Required binary: ${cc}"
	echo ""
	err "Please install manually:"

	case "${os}" in
	Darwin)
		local formula
		formula="$(get_brew_formula)"
		err "    brew tap ${BREW_TAP}"
		err "    brew install ${BREW_TAP}/${formula}"
		echo ""
		err 'Or set LINUX_CC to a custom cross-compiler (e.g. "zig cc -target ...")'
		;;
	*)
		err "    (install a gcc cross-compiler targeting linux for your architecture)"
		;;
	esac

	print_auto_install_hint
	exit 1
}

main "$@"
