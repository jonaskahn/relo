#!/bin/sh
# Runs before an .deb or .rpm is unpacked, on a fresh install and on an update
# alike. An update must not leave the old daemon holding the ports the new one
# is about to serve, so the old build is quiesced: its daemon is stopped and its
# ports are freed. The data is never touched here -- an update keeps it.
#
# A build that predates --force leaves processes behind, so those are stopped by
# name. The script always succeeds: a package manager must not refuse to install
# Relo because a process was still running.
set -u

if [ -x /usr/bin/relo ]; then
	/usr/bin/relo daemon stop --force || true
fi
pkill -x relo 2>/dev/null || true
pkill -x relo-desktop 2>/dev/null || true

exit 0