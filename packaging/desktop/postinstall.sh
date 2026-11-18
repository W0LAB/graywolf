#!/bin/sh
# graywolf-desktop postinstall. Does NOT create a service user or enable
# a systemd unit: the desktop app runs as the logged-in user.
udevadm control --reload-rules 2>/dev/null || true
udevadm trigger --subsystem-match=hidraw 2>/dev/null || true
update-desktop-database -q /usr/share/applications 2>/dev/null || true
gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor 2>/dev/null || true
cat <<'MSG'
Graywolf desktop installed. Start it from your applications menu
("Graywolf") or run: graywolf-desktop

Serial-port PTT needs your user in the dialout group (uucp on some
distributions). Add it, then log out and back in:
  sudo usermod -aG dialout "$USER"
USB PTT adapters (CM108 / AIOC) work for the logged-in user without
extra groups.
MSG
exit 0
