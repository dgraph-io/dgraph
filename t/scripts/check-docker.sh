#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

# ---- thresholds ----
MIN_DOCKER_MAJOR=29

MIN_COMPOSE_MAJOR=2
MIN_COMPOSE_MINOR=40
MIN_COMPOSE_PATCH=0

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

# Find docker binary
find_docker() {
	command -v docker 2>/dev/null
}

install_docker_linux() {
	if command -v apt-get &>/dev/null; then
		# Install prerequisites
		sudo apt-get update
		sudo apt-get install -y ca-certificates curl gnupg

		# Add Docker's official GPG key
		sudo install -m 0755 -d /etc/apt/keyrings
		curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
		sudo chmod a+r /etc/apt/keyrings/docker.gpg

		# Detect distro (works for Ubuntu and Debian)
		local distro
		if [[ -f /etc/os-release ]]; then
			# shellcheck source=/dev/null
			. /etc/os-release
			distro="${ID:-ubuntu}"
		else
			distro="ubuntu"
		fi

		# Add the repository
		echo \
			"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${distro} \
      $(. /etc/os-release && echo "${VERSION_CODENAME:-$(lsb_release -cs)}") stable" |
			sudo tee /etc/apt/sources.list.d/docker.list >/dev/null

		sudo apt-get update
		sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

		# Start and enable Docker service
		sudo systemctl start docker || true
		sudo systemctl enable docker || true

	elif command -v dnf &>/dev/null; then
		sudo dnf -y install dnf-plugins-core
		sudo dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
		sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
		sudo systemctl start docker || true
		sudo systemctl enable docker || true

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

	# Add current user to docker group to avoid needing sudo
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

print_install_instructions() {
	echo ""
	err "Docker is not installed"
	echo ""
	err "Please install Docker manually:"

	case "$(get_os)" in
	Linux)
		err "    apt (Ubuntu/Debian): see https://docs.docker.com/engine/install/ubuntu/"
		err "    dnf (Fedora):        see https://docs.docker.com/engine/install/fedora/"
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

check_docker_requirements() {
	# Check jq dependency for version parsing
	if ! command -v jq &>/dev/null; then
		err "jq not found in PATH (required for version parsing)"
		exit 1
	fi

	# Check Docker daemon is running
	if ! docker info &>/dev/null; then
		err "Docker daemon is not running"
		err "Please start Docker Desktop or the Docker service"
		exit 1
	fi

	# Fetch all info in one call
	local docker_info
	if ! docker_info="$(docker info --format '{{json .}}' 2>&1)"; then
		err "failed to get docker info: ${docker_info}"
		exit 1
	fi

	# 1) Parse and check Docker version
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

	# 2) Parse and check Docker Compose version
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

	# 3) Check memory
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

main() {
	# Check if docker is already available
	if find_docker &>/dev/null; then
		check_docker_requirements
		exit 0
	fi

	# Docker not found
	if [[ ${AUTO_INSTALL-} == "true" ]]; then
		install_docker

		# Re-check after install
		if find_docker &>/dev/null; then
			# On macOS, the daemon might not be running yet after cask install
			if docker info &>/dev/null; then
				check_docker_requirements
			else
				err "Docker installed but daemon not running yet"
				err "Please start Docker Desktop and re-run this script"
				exit 1
			fi
			exit 0
		else
			err "docker check still failing after installation"
			exit 1
		fi
	fi

	# No auto-install, fail with instructions
	print_install_instructions
	exit 1
}

main "$@"
