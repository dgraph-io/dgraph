#!/usr/bin/env bash
# shellcheck disable=SC2310
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

MIN_MAJOR=1
MIN_MINOR=13
INSTALL_CMD="go install gotest.tools/gotestsum@latest"

# Returns 0 if a.b >= min_major.min_minor
version_ok() {
	local maj=$1 min=$2
	((maj > MIN_MAJOR)) && return 0
	((maj < MIN_MAJOR)) && return 1
	((min >= MIN_MINOR))
}

check_gotestsum() {
	local gopath="${GOPATH:-${HOME}/go}"
	local gotestsum_bin="${gopath}/bin/gotestsum"

	if [[ ! -x ${gotestsum_bin} ]]; then
		return 1
	fi

	local version_out maj min
	if ! version_out="$("${gotestsum_bin}" --version 2>/dev/null)"; then
		return 1
	fi

	# Parse version (e.g., "gotestsum version 1.12.0" or just "1.12.0")
	if [[ ${version_out} =~ ([0-9]+)\.([0-9]+) ]]; then
		maj="${BASH_REMATCH[1]}"
		min="${BASH_REMATCH[2]}"
	else
		return 1
	fi

	if ! version_ok "${maj}" "${min}"; then
		warn "gotestsum ${maj}.${min} is below minimum required version ${MIN_MAJOR}.${MIN_MINOR}"
		return 1
	fi

	return 0
}

install_gotestsum() {
	if ! ${INSTALL_CMD}; then
		err "failed to install gotestsum"
		exit 1
	fi
}

main() {
	# Check go is available
	if ! command -v go &>/dev/null; then
		err "go not found in PATH"
		exit 1
	fi

	local gopath="${GOPATH:-${HOME}/go}"
	local gotestsum_bin="${gopath}/bin/gotestsum"

	# First check
	if check_gotestsum; then
		exit 0
	fi

	# Determine reason for failure
	local reason
	if [[ ! -x ${gotestsum_bin} ]]; then
		reason="gotestsum is not installed at ${gotestsum_bin}"
	else
		reason="gotestsum version is below minimum required (${MIN_MAJOR}.${MIN_MINOR})"
	fi

	# Auto-install if AUTO_INSTALL=true
	if [[ ${AUTO_INSTALL-} == "true" ]]; then
		install_gotestsum

		# Re-check after install
		if check_gotestsum; then
			exit 0
		else
			err "gotestsum check still failing after installation"
			exit 1
		fi
	fi

	# No auto-install, fail with instructions
	echo ""
	err "${reason}"
	echo ""
	err "Please install or upgrade gotestsum by running:"
	err "    ${INSTALL_CMD}"
	print_auto_install_hint
	exit 1
}

main "$@"
