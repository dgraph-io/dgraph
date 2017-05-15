#CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
CURRENT_BRANCH="release/v0.7.6"

VERSIONS=(
  'v0.7.6'
  'master'
  'v0.7.5'
  'v0.7.4'
)

joinVersions() {
	versions=$(printf ",%s" "${VERSIONS[@]}")
	echo ${versions:1}
}

VERSION_STRING=$(joinVersions)

export CURRENT_BRANCH=${CURRENT_BRANCH}
export VERSIONS=${VERSION_STRING}

HUGO_TITLE="Dgraph Doc - local" \
VERSIONS=${VERSION_STRING} \
CURRENT_BRANCH=${CURRENT_BRANCH} hugo server -w
