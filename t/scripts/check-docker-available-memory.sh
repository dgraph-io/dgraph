#!/usr/bin/env bash
# shellcheck disable=SC2310
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

# Minimum recommended memory in MB
REC_MEM_MB=8192

main() {
	local os mem_bytes mem_mb
	os="$(uname -s)"

	if [[ ${os} == "Linux" ]]; then
		# On Linux both Docker and Podman use host memory directly.
		# Read from /proc/meminfo instead of querying the container daemon.
		if [[ ! -r /proc/meminfo ]]; then
			warn "could not read /proc/meminfo"
			exit 0
		fi
		local mem_kb
		mem_kb="$(awk '/MemTotal/ {print $2}' /proc/meminfo 2>/dev/null)" || {
			warn "could not parse /proc/meminfo"
			exit 0
		}
		mem_bytes=$((mem_kb * 1024))
	else
		# macOS (Docker Desktop): memory is VM-limited; query via docker info.
		mem_bytes="$(docker info --format '{{.MemTotal}}' 2>/dev/null)" || {
			warn "could not query Docker memory (is Docker running?)"
			exit 0
		}
	fi

	if [[ -z ${mem_bytes} || ! ${mem_bytes} =~ ^[0-9]+$ ]]; then
		warn "could not parse memory info"
		exit 0
	fi

	mem_mb=$((mem_bytes / 1024 / 1024))

	if ((mem_mb >= REC_MEM_MB)); then
		exit 0
	fi

	# Memory is below recommended
	warn "Available memory ${mem_mb}MB is below recommended ${REC_MEM_MB}MB"
	warn "Some tests may fail with OOM errors"

	if [[ ${os} == "Darwin" ]]; then
		local settings_file="${HOME}/Library/Group Containers/group.com.docker/settings-store.json"

		if [[ ${AUTO_INSTALL-} == "true" ]]; then
			if [[ -f ${settings_file} ]] && command -v jq &>/dev/null; then
				local tmp
				tmp="$(mktemp)"
				if jq '.memoryMiB = 8192' "${settings_file}" >"${tmp}" 2>/dev/null; then
					cp "${tmp}" "${settings_file}"
					rm "${tmp}"
					warn "Updated Docker Desktop memory setting to 8192MB"
					warn "Please restart Docker Desktop for the change to take effect"
				else
					rm -f "${tmp}"
					warn "Could not update Docker Desktop settings automatically"
					warn "Please increase memory via: Docker Desktop → Settings → Resources → Memory"
				fi
			else
				warn "Please increase memory via: Docker Desktop → Settings → Resources → Memory"
			fi
		else
			warn "Please increase memory via: Docker Desktop → Settings → Resources → Memory"
		fi
	else
		warn "Consider adding more RAM to this machine or reducing test parallelism"
	fi

	# Always exit 0 — this is a warning, not a blocker
	exit 0
}

main "$@"
