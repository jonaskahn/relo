#!/bin/sh
# Runs before the package files are removed, which is the last moment the Relo
# binary can still be asked to do something: the daemon is stopped, the
# configured ports are freed, autostart is unregistered, and the data is removed
# only on a Debian purge. rpm has no purge, so `rpm -e` keeps the data and
# `relo uninstall --wipe` is the way to remove it.
#
# The script always succeeds: removing the package must work even when the
# binary is already gone or refuses.
set -u

if [ -x /usr/bin/relo ]; then
	if [ "${1:-remove}" = "purge" ]; then
		/usr/bin/relo uninstall --prepare --wipe || true
	else
		/usr/bin/relo uninstall --prepare || true
	fi
fi

exit 0