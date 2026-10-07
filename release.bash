#!/bin/bash
#
# Create a draft GitHub release from dist/*.zip. Run `make release` first.
# Requires: gh, jq, git, and sha256sum or shasum.
#
set -euo pipefail

for CMD in gh jq git; do
	command -v "$CMD" >/dev/null || { echo "error: $CMD is required" >&2; exit 1; }
done
if command -v sha256sum >/dev/null; then SHA="sha256sum"; else SHA="shasum -a 256"; fi

REPO_ID="$(basename "$(pwd)")"
RELEASE_TAG="v$(jq -r .version codemeta.json)"
RELEASE_NOTES="$(jq -r .releaseNotes codemeta.json)"
case "$RELEASE_TAG" in
	*[!0-9a-zA-Z._-]*) echo "error: version contains unexpected characters: $RELEASE_TAG" >&2; exit 1 ;;
esac
ls dist/*.zip >/dev/null 2>&1 || { echo "error: no dist/*.zip; run make release first" >&2; exit 1; }
echo "tag: ${RELEASE_TAG}, notes: ${RELEASE_NOTES}"

CHECKSUMS="${REPO_ID}-${RELEASE_TAG}-checksums.txt"
(cd dist && $SHA *.zip >"$CHECKSUMS")
echo "Checksums written to dist/${CHECKSUMS}"

read -r -p "Push release to GitHub with gh? (y/N) " YES_NO
if [ "$YES_NO" = "y" ]; then
	make save msg="prep for ${RELEASE_TAG}, ${RELEASE_NOTES}"
	gh release create "${RELEASE_TAG}" \
		--draft \
		--notes="${RELEASE_NOTES}" \
		dist/*.zip "dist/${CHECKSUMS}"
	echo "Now go to the repository's releases page and finalize the draft"
fi
