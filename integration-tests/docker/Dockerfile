#FROM ghcr.io/netcracker/qubership-docker-integration-tests:main

FROM polinakriukova/qubership-docker-integration-tests:latest

# Switch to root for installation of dependencies and packages
# Required for AWS EKS deployments and S3 integration scripts
USER root

ENV ROBOT_OUTPUT=/opt/robot/output \
    SERVICE_CHECKER_SCRIPT=${ROBOT_HOME}/consul_pods_checker.py

ENV STATUS_CUSTOM_RESOURCE_GROUP=apps
ENV STATUS_CUSTOM_RESOURCE_VERSION=v1
ENV STATUS_CUSTOM_RESOURCE_PLURAL=deployments
ENV STATUS_CUSTOM_RESOURCE_NAME=consul-integration-tests-runner

RUN echo "https://dl-cdn.alpinelinux.org/alpine/v3.23/main" > /etc/apk/repositories && \
    echo "https://dl-cdn.alpinelinux.org/alpine/v3.23/community" >> /etc/apk/repositories

RUN mkdir -p ${ROBOT_OUTPUT}

COPY integration-tests/docker/requirements.txt ${ROBOT_HOME}/requirements.txt
COPY integration-tests/docker/consul_pods_checker.py ${ROBOT_HOME}/consul_pods_checker.py
COPY integration-tests/robot ${ROBOT_HOME}


RUN set -x \
    && pip3 install -r ${ROBOT_HOME}/requirements.txt \
    && rm -rf /var/cache/apk/*

# Upgrade all tools to avoid vulnerabilities
RUN set -x && apk add --update --no-cache apk-tools && apk upgrade --no-cache --available

# Fix permissions for non-root user (1000:0)
# Added for AWS EKS security policies and S3 adapter scripts.
# When files are copied while USER is root, they get root ownership.
# This fixes ownership for:
# 1. AWS EKS strict security policies that restrict root execution
# 2. S3 upload scripts (inotifywait, s5cmd) from base image that need write access
# 3. Robot Framework to create output files and logs
RUN chown -R 1000:0 ${ROBOT_HOME} && chmod -R 775 ${ROBOT_HOME}

# Switch back to non-root user for AWS EKS security compliance

USER 1000:0

EXPOSE 8080
VOLUME ["${ROBOT_OUTPUT}"]
