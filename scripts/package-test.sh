#!/bin/sh
# Package test: unpack the customer archive into a clean directory, run preflight, load sample data,
# start the server, and check that health, login page and a portal page respond. Exit non-zero on any failure.
set -eu
archive=$1
work=$(mktemp -d)
trap 'kill $pid 2>/dev/null || true; rm -rf "$work"' EXIT
tar -xzf "$archive" -C "$work"
dir=$(find "$work" -maxdepth 1 -mindepth 1 -type d)
cd "$dir"
test -f README.md && test -f CHANGELOG.md && test -f THIRD_PARTY_LICENSES.txt && test -f sbom.json && test -f docs/install.md
export BRIEFRELAY_ENV=development BRIEFRELAY_DATA_DIR="$dir/data" BRIEFRELAY_ADDR=127.0.0.1:18080 BRIEFRELAY_BASE_URL=http://127.0.0.1:18080
./briefrelay check
./briefrelay seed
./briefrelay serve > server.log 2>&1 &
pid=$!
for i in $(seq 1 30); do
  curl -fsS http://127.0.0.1:18080/healthz > health.json 2>/dev/null && break
  sleep 0.5
done
grep -q '"status":"ok"' health.json
curl -fsS http://127.0.0.1:18080/login | grep -q 'Log in'
# Log in as the sample owner and open the project list.
curl -fsS -c jar -b jar -o /dev/null -w '%{http_code}\n' -d 'email=owner@demo.test&password=demo-owner-password' http://127.0.0.1:18080/login | grep -q 303
curl -fsS -b jar http://127.0.0.1:18080/projects | grep -q 'Blue Bakery website'
./briefrelay backup backup.tar.gz
test -s backup.tar.gz
echo "package test passed: $archive"
