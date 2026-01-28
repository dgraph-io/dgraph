#!/usr/bin/env bash
# Common functions for check scripts

err() { printf "ERROR: %s\n" "$*" >&2; }
warn() { printf "WARN:  %s\n" "$*" >&2; }

# Get the directory containing the scripts
SCRIPTS_DIR="$(dirname "${BASH_SOURCE[0]}")"

# Detect OS (Linux, Darwin, etc.)
get_os() {
	uname -s
}

# Install a package using the appropriate Linux package manager
# Usage: install_linux_pkg <apt-pkg> [<dnf-pkg>] [<yum-pkg>] [<pacman-pkg>]
# If only one argument is provided, it's used for all package managers
install_linux_pkg() {
	local apt_pkg="${1}"
	local dnf_pkg="${2:-${apt_pkg}}"
	local yum_pkg="${3:-${dnf_pkg}}"
	local pacman_pkg="${4:-${yum_pkg}}"

	if command -v apt-get &>/dev/null; then
		sudo apt-get update
		sudo apt-get install -y "${apt_pkg}"
	elif command -v dnf &>/dev/null; then
		sudo dnf install -y "${dnf_pkg}"
	elif command -v yum &>/dev/null; then
		sudo yum install -y "${yum_pkg}"
	elif command -v pacman &>/dev/null; then
		sudo pacman -S --noconfirm "${pacman_pkg}"
	else
		err "no supported package manager found (tried: apt, dnf, yum, pacman)"
		return 1
	fi
}

# Ensure Homebrew is installed and available (macOS)
ensure_brew() {
	if command -v brew &>/dev/null; then
		return 0
	fi

	if [[ ${AUTO_INSTALL-} != "true" ]]; then
		err "Homebrew is not installed"
		err "Please install Homebrew: https://brew.sh"
		err "Or re-run with AUTO_INSTALL=true"
		return 1
	fi

	/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

	# Extend PATH to find freshly installed brew, then source shellenv
	export PATH="/opt/homebrew/bin:/usr/local/bin:${PATH}"
	if command -v brew &>/dev/null; then
		eval "$("$(command -v brew)" shellenv)"
	else
		err "Homebrew installation failed"
		return 1
	fi
}

# Print the common "re-run with AUTO_INSTALL" message
print_auto_install_hint() {
	echo ""
	err "Or re-run with AUTO_INSTALL=true to install automatically."
}

# Run OS-specific installer
# Usage: run_os_installer <linux_func> <macos_func>
run_os_installer() {
	local linux_func="$1"
	local macos_func="$2"
	local os
	os="$(get_os)"

	case "${os}" in
	Linux)
		"${linux_func}"
		;;
	Darwin)
		"${macos_func}"
		;;
	*)
		err "unsupported operating system: ${os}"
		return 1
		;;
	esac
}
