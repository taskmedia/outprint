#!/bin/sh
# Runs every tier-2 test binary of the golden image and fails if any fails.
# GOLDEN_RUN, if set, is a -test.run pattern applied to every binary.
status=0
for test in worker.test golden.test; do
	if [ -n "$GOLDEN_RUN" ]; then
		"/usr/local/bin/$test" -test.run "$GOLDEN_RUN" "$@" || status=1
	else
		"/usr/local/bin/$test" "$@" || status=1
	fi
done
exit "$status"
