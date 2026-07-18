#!/usr/bin/env bash
# scripts/backup.sh — pg_dump | gzip -> object storage (ADR decision 33:
# nightly pg_dump to a versioned bucket, with rehearsed restores).
#
# Local dev: streams into the MinIO `pitlane-backups` bucket from
# docker-compose. Production (Phase D schedules this nightly): point the
# BACKUP_S3_* overrides at the R2 backups bucket.
#
# Retention: this script only ever WRITES timestamped objects — deletion is a
# bucket lifecycle policy (e.g. expire after 30 days), configured with the R2
# bucket in Phase D, keeping delete power out of the backup job itself.
set -euo pipefail
cd "$(dirname "$0")/.."

TIMESTAMP=$(date -u +%Y%m%dT%H%M%SZ)
OBJECT="pitlane-${TIMESTAMP}.sql.gz"

MC_IMAGE="minio/mc:RELEASE.2025-08-13T08-35-41Z"
BACKUP_BUCKET="${BACKUP_BUCKET:-pitlane-backups}"
S3_ENDPOINT="${BACKUP_S3_ENDPOINT:-http://minio:9000}"
S3_ACCESS_KEY="${BACKUP_S3_ACCESS_KEY:-pitlane}"
S3_SECRET_KEY="${BACKUP_S3_SECRET_KEY:-pitlane-dev-secret}"
COMPOSE_SERVICE="${BACKUP_COMPOSE_SERVICE:-postgres}"
PGUSER="${BACKUP_PGUSER:-pitlane}"
PGDB="${BACKUP_PGDB:-pitlane}"

echo "backup: pg_dump (${COMPOSE_SERVICE}/${PGDB}) | gzip -> s3://${BACKUP_BUCKET}/${OBJECT}"
docker compose exec -T "$COMPOSE_SERVICE" pg_dump -U "$PGUSER" -d "$PGDB" --no-owner --no-privileges \
  | gzip -9 \
  | docker run --rm -i --entrypoint sh --network pitlane_default "$MC_IMAGE" -c \
      "mc alias set target '$S3_ENDPOINT' '$S3_ACCESS_KEY' '$S3_SECRET_KEY' >/dev/null \
       && mc pipe 'target/${BACKUP_BUCKET}/${OBJECT}'"
echo "backup: done -> ${OBJECT}"
