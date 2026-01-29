#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

# Parse required Go version from go.mod
get_required_go_version() {
	local repo_root
	repo_root="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel 2>/dev/null)" || {
		err "could not find git repository root"
		return 1
	}

	local go_mod="${repo_root}/go.mod"
	if [[ ! -f ${go_mod} ]]; then
		err "go.mod not found at ${go_mod}"
		return 1
	fi

	# Extract version from "go X.Y.Z" line
	local version
	version=$(grep -E '^go [0-9]+\.[0-9]+' "${go_mod}" | awk '{print $2}')
	if [[ -z ${version} ]]; then
		err "could not parse go version from go.mod"
		return 1
	fi

	echo "${version}"
}

MIN_GO_VERSION="$(get_required_go_version)" || exit 1

find_go() {
	command -v go 2>/dev/null
}

# Check if installed version meets minimum requirement
version_ok() {
	local current="$1"
	local required="${MIN_GO_VERSION}"
	local lowest
	lowest=$(printf '%s\n%s\n' "${current}" "${required}" | sort -V | head -n1)
	[[ ${lowest} == "${required}" ]]
}

install_go_linux() {
	local version="${MIN_GO_VERSION}"
	local arch
	arch="$(dpkg --print-architecture 2>/dev/null || uname -m)"

	case "${arch}" in
	x86_64 | amd64) arch="amd64" ;;
	aarch64 | arm64) arch="arm64" ;;
	*)
		err "unsupported architecture: ${arch}"
		exit 1
		;;
	esac

	local tarball="go${version}.linux-${arch}.tar.gz"
	local url="https://go.dev/dl/${tarball}"

	curl -fsSL "${url}" -o "/tmp/${tarball}"
	sudo rm -rf /usr/local/go
	sudo tar -C /usr/local -xzf "/tmp/${tarball}"
	rm "/tmp/${tarball}"

	export PATH="/usr/local/go/bin:${PATH}"
}

install_go_macos() {
	ensure_brew || exit 1
	brew install go
}

install_go() {
	run_os_installer install_go_linux install_go_macos
}

check_go_version() {
	local go_bin="$1"
	local version_out
	version_out="$("${go_bin}" version 2>/dev/null)"

	if [[ ${version_out} =~ go([0-9]+\.[0-9]+\.?[0-9]*) ]]; then
		local current="${BASH_REMATCH[1]}"
		if version_ok "${current}"; then
			return 0
		else
			err "Go version ${current} is below minimum required (${MIN_GO_VERSION})"
			return 1
		fi
	else
		err "could not parse Go version from: ${version_out}"
		return 1
	fi
}

main() {
	local go_bin
	if go_bin="$(find_go)" && check_go_version "${go_bin}"; then
		exit 0
	fi

	if [[ ${AUTO_INSTALL-} == "true" ]]; then
		install_go
		if go_bin="$(find_go)" && check_go_version "${go_bin}"; then
			exit 0
		else
			err "go check still failing after installation"
			exit 1
		fi
	fi

	echo ""
	if find_go &>/dev/null; then
		err "Go is installed but version is below minimum required (${MIN_GO_VERSION})"
	else
		err "Go is not installed"
	fi
	echo ""
	err "Please install Go ${MIN_GO_VERSION} or higher:"

	case "$(get_os)" in
	Linux)
		err "    See: https://go.dev/doc/install"
		;;
	Darwin)
		err "    brew install go"
		err "    Or download from: https://go.dev/doc/install"
		;;
	*)
		err "    See: https://go.dev/doc/install"
		;;
	esac

	print_auto_install_hint
	exit 1
}

main "$@"
