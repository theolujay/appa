#!/bin/bash

# Usage: ./update-secret.sh secret-name secret-value [stack-name]

set -euo pipefail

SECRET_NAME=$1
SECRET_VALUE=$2
STACK_NAME=${3:-vmlc-prod}

if [ -z "$SECRET_NAME" ] || [ -z "$SECRET_VALUE" ]; then
    echo "Usage: $0 <secret_name> <secret_value> [stack_name]"
    exit 1
fi

TEMP_SECRET_NAME="${SECRET_NAME}-temp"

echo "1/8:Creating temporary secret: $TEMP_SECRET_NAME..."
printf "%s" "$SECRET_VALUE" | docker secret create "$TEMP_SECRET_NAME" -

echo "2/8:Updating services to use temporary secret..."
for service in db-migrate django celery_beat celery_worker; do
    echo "  → Updating ${STACK_NAME}_${service}..."
    docker service update \
      --secret-rm "$SECRET_NAME" \
      --secret-add "source=$TEMP_SECRET_NAME,target=$SECRET_NAME" \
      "${STACK_NAME}_${service}" > /dev/null
done

echo "3/8:Waiting for services to stabilize..."
sleep 10

echo "4/8:Removing old secret..."
docker secret rm "$SECRET_NAME"

echo "5/8:Creating new secret with original name..."
printf "%s" "$SECRET_VALUE" | docker secret create "$SECRET_NAME" -

echo "6/8:Updating services back to original secret name..."
for service in db-migrate django celery_beat celery_worker grafana; do
    echo "  → Updating ${STACK_NAME}_${service}..."
    docker service update \
      --secret-rm "$TEMP_SECRET_NAME" \
      --secret-add "source=$SECRET_NAME,target=$SECRET_NAME" \
      "${STACK_NAME}_${service}" > /dev/null
done

echo "7/8:Waiting for services to stabilize..."
sleep 10

echo "8/8:Cleaning up temporary secret..."
docker secret rm "$TEMP_SECRET_NAME"

echo "Done: Secret $SECRET_NAME updated successfully."
echo "   Services are now using the new value with the original name."