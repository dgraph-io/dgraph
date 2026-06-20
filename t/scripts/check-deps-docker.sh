#!/usr/bin/env bash
# shellcheck disable=SC1091,SC2310,SC2312,SC2329
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

# ---- version thresholds ----
MIN_DOCKER_MAJOR=29

MIN_COMPOSE_MAJOR=2
MIN_COMPOSE_MINOR=40
MIN_COMPOSE_PATCH=0

MIN_PODMAN_MAJOR=4
MIN_PODMAN_MINOR=0

MIN_MEM_MB=4
REC_MEM_MB=8

# Compare semver: returns 0 if a >= b
semver_ge() {
	local aMaj=$1 aMin=$2 aPat=$3 bMaj=$4 bMin=$5 bPat=$6
	((aMaj > bMaj)) && return 0
	((aMaj < bMaj)) && return 1
	((aMin > bMin)) && return 0
	((aMin < bMin)) && return 1
	((aPat >= bPat))
}

find_docker() {
	command -v docker 2>/dev/null
}

find_podman() {
	command -v podman 2>/dev/null
}

# Returns "docker", "podman", or "none".
detect_runtime() {
	if find_docker &>/dev/null; then
		local ver
		ver="$(docker --version 2>/dev/null | tr '[:upper:]' '[:lower:]')"
		if [[ "${ver}" == *"podman"* ]]; then
			echo "podman"
		else
			echo "docker"
		fi
	elif find_podman &>/dev/null; then
		echo "podman"
	else
		echo "none"
	fi
}

# ---- Docker installation helpers ----

install_docker_linux() {
	if command -v apt-get &>/dev/null; then
		sudo apt-get update
		sudo apt-get install -y ca-certificates curl gnupg

		sudo install -m 0755 -d /etc/apt/keyrings
		curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
		sudo chmod a+r /etc/apt/keyrings/docker.gpg

		local distro
		if [[ -f /etc/os-release ]]; then
			# shellcheck source=/dev/null
			. /etc/os-release
			distro="${ID:-ubuntu}"
		else
			distro="ubuntu"
		fi

		echo \
			"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${distro} \
  $(. /etc/os-release && echo "${VERSION_CODENAME:-$(lsb_release -cs)}") stable" |
			sudo tee /etc/apt/sources.list.d/docker.list >/dev/null

		sudo apt-get update
		sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

		sudo systemctl start docker || true
		sudo systemctl enable docker || true

	elif command -v dnf &>/dev/null; then
		# Fedora 38+ and RHEL 9+ do not support Docker CE from the official
		# Docker repo. Use Podman with the Docker compatibility layer instead.
		install_podman_linux

	elif command -v yum &>/dev/null; then
		sudo yum install -y yum-utils
		sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
		sudo yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
		sudo systemctl start docker || true
		sudo systemctl enable docker || true

	elif command -v pacman &>/dev/null; then
		sudo pacman -S --noconfirm docker docker-compose
		sudo systemctl start docker || true
		sudo systemctl enable docker || true

	else
		err "no supported package manager found (tried: apt, dnf, yum, pacman)"
		exit 1
	fi

	if [[ -n ${SUDO_USER-} ]]; then
		sudo usermod -aG docker "${SUDO_USER}" || true
	elif [[ -n ${USER-} ]] && [[ ${USER} != "root" ]]; then
		sudo usermod -aG docker "${USER}" || true
	fi
}

install_docker_macos() {
	ensure_brew || exit 1
	brew install --cask docker
}

install_docker() {
	run_os_installer install_docker_linux install_docker_macos
}

# ---- Podman installation helpers ----

install_podman_linux() {
	if command -v dnf &>/dev/null; then
		# Fedora / RHEL: podman-docker provides the docker CLI shim,
		# podman-compose provides docker-compose-compatible functionality.
		sudo dnf install -y podman podman-docker podman-compose
		# Enable the Podman socket so the Docker SDK can connect to it.
		systemctl --user enable --now podman.socket 2>/dev/null || true

	elif command -v apt-get &>/dev/null; then
		sudo apt-get update
		sudo apt-get install -y podman podman-compose
		systemctl --user enable --now podman.socket 2>/dev/null || true

	elif command -v pacman &>/dev/null; then
		sudo pacman -S --noconfirm podman podman-compose
		systemctl --user enable --now podman.socket 2>/dev/null || true

	else
		err "no supported package manager found (tried: dnf, apt, pacman)"
		exit 1
	fi
}

install_podman_macos() {
	ensure_brew || exit 1
	brew install podman podman-compose
}

install_podman() {
	run_os_installer install_podman_linux install_podman_macos
}

# ---- Install instructions ----

print_docker_install_instructions() {
	echo ""
	err "Docker is not installed"
	echo ""
	err "Please install Docker manually:"

	case "$(get_os)" in
	Linux)
		err "    apt (Ubuntu/Debian): see https://docs.docker.com/engine/install/ubuntu/"
		err "    dnf (Fedora/RHEL):   install Podman instead — see print_podman_install_instructions"
		err "    yum (CentOS/RHEL):   see https://docs.docker.com/engine/install/centos/"
		err "    pacman (Arch):       sudo pacman -S docker docker-compose"
		;;
	Darwin)
		err "    brew install --cask docker"
		err "    Or download from: https://www.docker.com/products/docker-desktop"
		;;
	*)
		err "    See: https://docs.docker.com/get-docker/"
		;;
	esac

	print_auto_install_hint
}

print_podman_install_instructions() {
	echo ""
	err "Podman is not installed (required on Fedora/RHEL where Docker CE is unavailable)"
	echo ""
	err "Please install Podman manually:"

	case "$(get_os)" in
	Linux)
		err "    dnf (Fedora/RHEL):   sudo dnf install -y podman podman-docker podman-compose"
		err "                          systemctl --user enable --now podman.socket"
		err "    apt (Ubuntu/Debian): sudo apt-get install -y podman podman-compose"
		err "    pacman (Arch):       sudo pacman -S podman podman-compose"
		;;
	Darwin)
		err "    brew install podman podman-compose"
		;;
	*)
		err "    See: https://podman.io/getting-started/installation"
		;;
	esac

	print_auto_install_hint
}

# ---- Docker requirement checks ----

check_docker_requirements() {
	if ! command -v jq &>/dev/null; then
		err "jq not found in PATH (required for version parsing)"
		exit 1
	fi

	if ! docker info &>/dev/null; then
		err "Docker daemon is not running"
		err "Please start Docker Desktop or the Docker service"
		exit 1
	fi

	local docker_info
	if ! docker_info="$(docker info --format '{{json .}}' 2>&1)"; then
		err "failed to get docker info: ${docker_info}"
		exit 1
	fi

	local docker_ver docker_maj docker_min docker_pat
	docker_ver="$(jq -r '.ServerVersion // empty' <<<"${docker_info}")"
	if [[ -z ${docker_ver} ]]; then
		err "could not get ServerVersion from docker info"
		exit 1
	fi
	IFS='.' read -r docker_maj docker_min docker_pat <<<"${docker_ver%%-*}"
	docker_pat="${docker_pat:-0}"

	if ((docker_maj < MIN_DOCKER_MAJOR)); then
		err "Docker version ${docker_maj}.${docker_min}.${docker_pat} is below minimum (need >= ${MIN_DOCKER_MAJOR})"
		exit 1
	fi

	local compose_ver compose_maj compose_min compose_pat
	compose_ver="$(jq -r '.ClientInfo.Plugins[] | select(.Name == "compose") | .Version // empty' <<<"${docker_info}" | sed 's/^v//')"
	if [[ -z ${compose_ver} ]]; then
		err "Docker Compose plugin not found (is Compose v2 installed?)"
		exit 1
	fi
	IFS='.' read -r compose_maj compose_min compose_pat <<<"${compose_ver%%-*}"
	compose_pat="${compose_pat:-0}"

	if ! semver_ge "${compose_maj}" "${compose_min}" "${compose_pat}" \
		"${MIN_COMPOSE_MAJOR}" "${MIN_COMPOSE_MINOR}" "${MIN_COMPOSE_PATCH}"; then
		err "Docker Compose ${compose_maj}.${compose_min}.${compose_pat} is below minimum (need >= ${MIN_COMPOSE_MAJOR}.${MIN_COMPOSE_MINOR}.${MIN_COMPOSE_PATCH})"
		exit 1
	fi

	local mem_bytes mem_mb
	mem_bytes="$(jq -r '.MemTotal // empty' <<<"${docker_info}")"
	if [[ -z ${mem_bytes} || ! ${mem_bytes} =~ ^[0-9]+$ ]]; then
		err "could not get MemTotal from docker info"
		exit 1
	fi
	mem_mb=$((mem_bytes / 1024 / 1024))

	if ((mem_mb < MIN_MEM_MB)); then
		err "Docker memory ${mem_mb}MB is below minimum (need >= ${MIN_MEM_MB}MB)"
		exit 1
	fi
	if ((mem_mb < REC_MEM_MB)); then
		warn "Docker memory ${mem_mb}MB is below recommended (>= ${REC_MEM_MB}MB)"
	fi
}

# ---- Podman requirement checks ----

check_podman_requirements() {
	# Determine the actual podman binary (might be via docker shim or directly).
	local podman_bin="podman"
	if ! command -v podman &>/dev/null; then
		# docker is a podman shim but podman binary itself isn't in PATH — still OK.
		if ! docker --version &>/dev/null; then
			err "Neither podman nor docker (podman shim) found in PATH"
			exit 1
		fi
		podman_bin="docker"
	fi

	local ver major minor
	ver="$("${podman_bin}" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)"
	if [[ -z ${ver} ]]; then
		err "Could not determine Podman version"
		exit 1
	fi
	IFS='.' read -r major minor _ <<<"${ver}"

	if ! semver_ge "${major}" "${minor}" "0" \
		"${MIN_PODMAN_MAJOR}" "${MIN_PODMAN_MINOR}" "0"; then
		err "Podman ${ver} is below minimum (need >= ${MIN_PODMAN_MAJOR}.${MIN_PODMAN_MINOR})"
		exit 1
	fi

	# Check that compose support is available.
	if ! podman compose version &>/dev/null 2>&1 &&
		! command -v podman-compose &>/dev/null; then
		err "No Podman compose support found."
		err "Install podman-compose:  sudo dnf install -y podman-compose"
		err "                    or:  pip3 install podman-compose"
		exit 1
	fi

	# Check memory using /proc/meminfo (Podman uses host memory on Linux).
	local mem_kb mem_mb
	if [[ -r /proc/meminfo ]]; then
		mem_kb="$(awk '/MemTotal/ {print $2}' /proc/meminfo)"
		mem_mb=$((mem_kb / 1024))
		if ((mem_mb < MIN_MEM_MB * 1024)); then
			err "System memory ${mem_mb}MB is below minimum (need >= $((MIN_MEM_MB * 1024))MB)"
			exit 1
		fi
		if ((mem_mb < REC_MEM_MB * 1024)); then
			warn "System memory ${mem_mb}MB is below recommended (>= $((REC_MEM_MB * 1024))MB)"
		fi
	fi
}

main() {
	local runtime
	runtime="$(detect_runtime)"

	case "${runtime}" in
	docker)
		check_docker_requirements
		exit 0
		;;
	podman)
		check_podman_requirements
		exit 0
		;;
	none)
		# Neither docker nor podman found.
		if [[ ${AUTO_INSTALL-} == "true" ]]; then
			if command -v dnf &>/dev/null; then
				# Fedora/RHEL: use Podman.
				install_podman
				check_podman_requirements
			else
				install_docker
				if find_docker &>/dev/null && docker info &>/dev/null; then
					check_docker_requirements
				else
					err "docker check still failing after installation"
					exit 1
				fi
			fi
			exit 0
		fi
		# No auto-install: decide which instructions to show.
		if command -v dnf &>/dev/null; then
			print_podman_install_instructions
		else
			print_docker_install_instructions
		fi
		exit 1
		;;
	esac
}

main "$@"
