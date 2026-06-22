#!/bin/sh
set -eu

if [ "$#" -gt 0 ]; then
  case "$1" in
    migrate|docker-bootstrap|start|stop|resetadmin|uninstall|uri|setting|admin|materialize-core-config|cleanup-core-config)
      exec /app/kwor "$@"
      ;;
    -h|--help|-v|version)
      exec /app/kwor "$@"
      ;;
  esac
fi

/app/kwor migrate
/app/kwor docker-bootstrap

if [ "$#" -gt 0 ]; then
  exec /app/kwor "$@"
fi

exec /app/kwor
