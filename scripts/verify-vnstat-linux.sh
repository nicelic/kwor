#!/bin/sh
# Read-only preflight and manual acceptance checklist for Debian/Ubuntu hosts.
# This script deliberately does not install, stop, disable, or delete anything.
set -eu

if [ "$(uname -s)" != "Linux" ]; then
  echo "FAIL: this acceptance checklist must run on Linux" >&2
  exit 1
fi

if [ ! -r /etc/os-release ]; then
  echo "FAIL: /etc/os-release is unavailable" >&2
  exit 1
fi

. /etc/os-release
case "${ID:-}" in
  debian|ubuntu)
    echo "PASS: supported acceptance family: ${ID} ${VERSION_ID:-unknown}"
    ;;
  *)
    echo "WARN: this checklist is intended for Debian/Ubuntu; found ${ID:-unknown} ${VERSION_ID:-unknown}"
    ;;
esac

if command -v apt-get >/dev/null 2>&1; then
  echo "PASS: apt-get is available"
else
  echo "FAIL: apt-get is unavailable; system-source acceptance cannot proceed" >&2
  exit 1
fi

if command -v apt-cache >/dev/null 2>&1 && apt-cache show vnstat >/dev/null 2>&1; then
  echo "PASS: apt metadata exposes vnstat"
else
  echo "WARN: vnstat package metadata is unavailable; verify configured repositories before testing"
fi

if command -v systemctl >/dev/null 2>&1; then
  echo "INFO: systemctl is available"
else
  echo "INFO: no systemctl; test the verified SysV path instead"
fi

echo
echo "Manual acceptance sequence (perform from the panel UI on a disposable host):"
echo "1. Start or restart the panel beside an external vnstatd; verify it is neither adopted nor stopped and overview polling does not enumerate it."
echo "2. Install via 系统软件源; verify status becomes panel-installed and ownership.json exists under the panel data directory."
echo "3. Reinstall via GitHub 官方源码包; verify DESTDIR failure leaves the existing installation untouched."
echo "4. Restore/copy only the database manifest; verify the UI shows 未由本面板安装 and delete is disabled."
echo "5. Run a non-default /opt/.../vnstatd under a verified systemd unit; click 下载 / 安装 and verify its PID and verified unit stop/disable while its /opt files remain."
echo "6. Verify a healthy panel-managed vnstat shows no conflict; force its start to fail beside a running external daemon and verify the UI shows path, PID, service, and conflict reason."
echo "7. Make a verified daemon refuse to exit; click 删除 and verify no package/file removal occurs."
echo "8. Delete a valid managed residual (binary missing); verify only credential-listed files and the three allowed data directories are removed."
echo "9. Uninstall the panel with a valid managed manifest; verify its package or source files, service, data, manifest, and ownership evidence are removed. Repeat with an unverified manifest and verify host vnstat files remain."
echo "10. Verify no cron, rc.local, shell profile, or unknown service was changed."
