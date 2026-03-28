#!/bin/sh

set -eu

APP_BIN="/go/cmd/writefreely/writefreely"
BOOTSTRAP_MARKER="/go/keys/.docker-bootstrap-done"

admin_user="${WRITEFREELY_ADMIN_USER:-admin}"
admin_pass="${WRITEFREELY_ADMIN_PASS:-adminpass}"

mkdir -p /go/keys

"$APP_BIN" --gen-keys

if [ ! -f "$BOOTSTRAP_MARKER" ]; then
	"$APP_BIN" db init
	"$APP_BIN" user create --admin "${admin_user}:${admin_pass}"
	touch "$BOOTSTRAP_MARKER"
else
	"$APP_BIN" db migrate
fi

exec "$APP_BIN"
