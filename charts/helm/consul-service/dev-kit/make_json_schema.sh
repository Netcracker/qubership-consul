#!/usr/bin/env bash
# Refer to https://git.netcracker.com/Personal.Streaming.Platform/values-schema-generator#user-guide
# to install nc-schema-gen
ROOT_PATH=../../../../
nc-schema-gen --leveling --skip-type-mismatch --off-array-list-mismatch-log \
  -n opensearch-service \
  -d $ROOT_PATH/docs/public/installation.md \
  -v $ROOT_PATH/charts/helm/consul-service/values.yaml \
  -s $ROOT_PATH/charts/helm/consul-service/values.schema.json \
  -r $ROOT_PATH/charts/helm/consul-service/values.rules.json
