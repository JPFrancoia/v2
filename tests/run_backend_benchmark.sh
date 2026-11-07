#!/bin/sh
# SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
set -eu

cd "$(dirname "$0")/.."
image=${BENCHMARK_IMAGE:-miniflux-backend-benchmark:local}
entries=${BENCHMARK_ENTRIES:-100000}
samples=${BENCHMARK_SAMPLES:-11}
output=${BENCHMARK_OUTPUT:-backend-benchmark.json}
python3 -c 'import sys; n,r=map(int,sys.argv[1:]); assert 1000 <= n <= 1000000 and 1 <= r <= 100' "$entries" "$samples"
docker image inspect "$image" > /dev/null
run=$(python3 -c 'import secrets; print(secrets.token_hex(8))')
network=miniflux-bench-$run
database=$network-db
app=$network-app
network_created=0
database_created=0
app_created=0
temporary=$(mktemp -d)
cleanup() {
    if [ "$app_created" = 1 ]; then docker rm -fv "$app" > /dev/null; fi
    if [ "$database_created" = 1 ]; then docker rm -fv "$database" > /dev/null; fi
    if [ "$network_created" = 1 ]; then docker network rm "$network" > /dev/null; fi
    rm -rf "$temporary"
}
trap cleanup EXIT HUP INT TERM
port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
password=$(python3 -c 'import secrets; print(secrets.token_urlsafe(24))')
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj '/CN=Miniflux local benchmark' \
    -addext 'basicConstraints=critical,CA:TRUE' -addext 'subjectAltName=DNS:localhost,IP:127.0.0.1' \
    -keyout "$temporary/server.key" -out "$temporary/server.crt" > /dev/null 2>&1
chmod 600 "$temporary/server.key"
docker network create "$network" > /dev/null
network_created=1
docker run -d --name "$database" --network "$network" \
    -e POSTGRES_DB=miniflux_ui_benchmark -e POSTGRES_USER=benchmark \
    -e "POSTGRES_PASSWORD=$password" postgres:17-alpine > /dev/null
database_created=1
attempt=0
until docker exec "$database" pg_isready -U benchmark -d miniflux_ui_benchmark > /dev/null 2>&1; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 60 ] || { docker logs "$database"; exit 1; }
    sleep 0.5
done
cat > "$temporary/local.env" <<EOF
DATABASE_URL=postgres://benchmark:$password@$database/miniflux_ui_benchmark?sslmode=disable
RUN_MIGRATIONS=1
CREATE_ADMIN=1
ADMIN_USERNAME=benchmark
ADMIN_PASSWORD=$password
BASE_URL=https://localhost:$port
LISTEN_ADDR=0.0.0.0:8080
CERT_FILE=/tls/server.crt
KEY_FILE=/tls/server.key
POLLING_FREQUENCY=1440
TO_REVIEW_RAW_RELEVANCE_TARGET=50
LOG_LEVEL=error
EOF
chmod 600 "$temporary/local.env"
docker run -d --name "$app" --network "$network" --user "$(id -u):$(id -g)" \
    --env-file "$temporary/local.env" -v "$temporary:/tls:ro" \
    -p "127.0.0.1:$port:8080" "$image" > /dev/null
app_created=1
attempt=0
until curl --silent --fail --cacert "$temporary/server.crt" "https://localhost:$port/healthcheck" > /dev/null; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 60 ] || { docker logs "$app"; exit 1; }
    sleep 0.5
done
docker exec -i "$database" psql -Xq -U benchmark -d miniflux_ui_benchmark \
    -v "entries=$entries" < tests/backend_benchmark.sql > /dev/null
BENCHMARK_PASSWORD=$password \
BENCHMARK_IMAGE_ID=$(docker image inspect "$image" --format '{{.Id}}') \
BENCHMARK_DATABASE_IMAGE_ID=$(docker image inspect postgres:17-alpine --format '{{.Id}}') \
    python3 tests/backend_benchmark.py --url "https://localhost:$port" \
    --ca-file "$temporary/server.crt" --entries "$entries" --samples "$samples" \
    --label "$image" --output "$output"
