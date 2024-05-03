#!/bin/sh

set -e

while :; do
	out=$(go run ./cmd/tools/getproto "$@")
	ret=$?
	if [ "$out" != "<rerun>" ]; then
		exit $ret
	fi
done
