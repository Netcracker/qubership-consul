FROM ghcr.io/netcracker/qubership-docker-integration-tests:main

ENV ROBOT_OUTPUT=/opt/robot/output \
    SERVICE_CHECKER_SCRIPT=${ROBOT_HOME}/consul_pods_checker.py

ENV STATUS_CUSTOM_RESOURCE_GROUP=apps
ENV STATUS_CUSTOM_RESOURCE_VERSION=v1
ENV STATUS_CUSTOM_RESOURCE_PLURAL=deployments
ENV STATUS_CUSTOM_RESOURCE_NAME=consul-integration-tests-runner

RUN mkdir -p ${ROBOT_OUTPUT}

COPY integration-tests/docker/requirements.txt ${ROBOT_HOME}/requirements.txt
COPY integration-tests/docker/consul_pods_checker.py ${ROBOT_HOME}/consul_pods_checker.py
COPY integration-tests/robot ${ROBOT_HOME}


RUN set -x \
    && pip3 install -r ${ROBOT_HOME}/requirements.txt \
    && rm -rf /var/cache/apk/*

# Upgrade all tools to avoid vulnerabilities
RUN set -x && apk add --update --no-cache apk-tools && apk upgrade --no-cache --available

USER 1000:0

EXPOSE 8080
VOLUME ["${ROBOT_OUTPUT}"]
