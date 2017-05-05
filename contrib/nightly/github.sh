# Ported over beautiful code from https://raw.githubusercontent.com/neovim/bot-ci/master/ci/common/github-api.sh.

# Exit if there's an error message.
# ${1}: Additional information about API call.
_check_gh_error() {
  local response="${1}"
  local error_message="$(echo "${response}" | jq -r '.message?')"
  if [[ -n "${error_message}" && "${error_message}" != 'null' ]]; then
    >&2 echo "Error ${2}: ${error_message}."
    return 1
  else
    echo "${response}"
  fi
}

# Send a request to the Github API.
# ${1}: API endpoint.
# ${2}: HTTP verb (default: GET).
send_gh_api_request() {
  local endpoint="${1}"
  local verb="${2:-GET}"

  local response="$(curl -v -H "Accept: application/vnd.github.v3+json" \
    -H "User-Agent: travis" \
    -u "${GH_TOKEN}:x-oauth-basic" \
    -X ${verb} \
    https://api.github.com/${endpoint} \
    2>/dev/null)"
  _check_gh_error "${response}" "calling ${endpoint} (${verb})"
}

# Send a data request to the Github API.
# ${1}: API endpoint.
# ${2}: HTTP verb.
# ${3}: JSON data.
send_gh_api_data_request() {
  local endpoint="${1}"
  local verb="${2}"
  local data="${3}"

  local response="$(curl -H "Accept: application/vnd.github.v3+json" \
    -H "User-Agent: travis" \
    -u "${GH_TOKEN}:x-oauth-basic" \
    -X ${verb} \
    -d "${data}" \
    https://api.github.com/${endpoint} \
    2>/dev/null)"
  _check_gh_error "${response}" "calling ${endpoint} (${verb})"
}

# Upload an asset to a Github release.
# ${1}: Local path to file to upload.
# ${2}: Desired file name on Github.
# ${3}: Repository to upload the asset to.
# ${4}: Release ID to upload the asset to.
upload_release_asset() {
  local file="${1}"
  local file_name="${2}"
  local repository="${3}"
  local release_id="${4}"
  local mime_type="$(file --mime-type -b "${file}")"

  local response="$(curl -H "Accept: application/vnd.github.v3+json" \
    -H "User-Agent: travis" \
    -H "Content-Type: ${mime_type}" \
    -u "${GH_TOKEN}:x-oauth-basic" \
    -T "${file}"\
    https://uploads.github.com/repos/${repository}/releases/${release_id}/assets?name=${file_name} \
    2>/dev/null)"
  _check_gh_error "${response}" 'uploading release assets'
}
