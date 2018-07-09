#!/bin/bash
if pkg-config libapparmor 2> /dev/null ; then
	# Travis CI does not support AppArmor, so we cannot run tests there.
	if [ -z "$TRAVIS" ]; then
		echo apparmor
	fi
fi
