#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=checkhelper.sh
source "$(dirname "${BASH_SOURCE[0]}")/checkhelper.sh"

# If run directly, just check/install brew
ensure_brew
