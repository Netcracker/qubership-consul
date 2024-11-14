#!/bin/sh

TARGET_DIR=target
CHARTS_NAME=consul-service-helm-charts
DOCKER_FILE=integration-tests/docker/Dockerfile

mkdir -p ${TARGET_DIR}

echo "Build docker image"
for docker_image_name in ${DOCKER_NAMES}; do
  echo "Docker image name: $docker_image_name"
  docker build \
    --file=${DOCKER_FILE} \
    --pull \
    -t ${docker_image_name} \
    .
done

mkdir -p deployments/charts/consul-service
cp -R ./charts/helm/consul-service/* deployments/charts/consul-service
cp ./charts/deployment-configuration.json deployments/deployment-configuration.json

echo "Archive artifacts"
zip -r ${TARGET_DIR}/${CHARTS_NAME}.zip \
  charts/helm/consul-service
