#!/usr/bin/env bash
# shellcheck disable=SC2310
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

# Minimum recommended Docker memory in MB
REC_MEM_MB=8192

main() {
	# Query Docker memory (Docker must be running — guaranteed by Make prereq on check-deps-docker)
	local mem_bytes
	mem_bytes="$(docker info --format '{{.MemTotal}}' 2>/dev/null)" || {
		warn "could not query Docker memory (is Docker running?)"
		exit 0
	}

	if [[ -z ${mem_bytes} || ! ${mem_bytes} =~ ^[0-9]+$ ]]; then
		warn "could not parse Docker memory info"
		exit 0
	fi

	local mem_mb=$((mem_bytes / 1024 / 1024))

	if ((mem_mb >= REC_MEM_MB)); then
		exit 0
	fi

	# Memory is below recommended
	warn "Docker memory ${mem_mb}MB is below recommended ${REC_MEM_MB}MB"
	warn "Some tests may fail with OOM errors"

	local os
	os="$(uname -s)"

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
		# Linux — Docker uses host memory directly
		warn "Consider adding more RAM to this machine or reducing test parallelism"
	fi

	# Always exit 0 — this is a warning, not a blocker
	exit 0
}

main "$@"
