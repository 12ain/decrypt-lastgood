#!/usr/bin/env bash
# Uploads release assets, retrying the transient network failures runners see:
# api.github.com sometimes refuses a connection for a few seconds, and a release
# job that has already built everything should not die for that.
#
#   ./scripts/upload-assets.sh <tag> <file>...
set -euo pipefail

tag=$1
shift
if [ $# -eq 0 ]; then
	echo "usage: $0 <tag> <file>..." >&2
	exit 2
fi

for attempt in 1 2 3 4 5; do
	if gh release upload "$tag" "$@" --clobber; then
		exit 0
	fi
	if [ "$attempt" -eq 5 ]; then
		echo "giving up after $attempt attempts" >&2
		exit 1
	fi
	delay=$((attempt * 10))
	echo "upload failed (attempt $attempt), retrying in ${delay}s" >&2
	sleep "$delay"
done
