#!/usr/bin/env bash

set -e

while :; do
	out=$(go run ./cmd/tools/getproto)
	if [[ $out = "<rerun>" ]]; then
		continue
	elif [[ -d $out ]]; then
		echo "$out"
		break
	else
		echo "getproto error: $out" 1>&2
		exit 1
	fi
done
