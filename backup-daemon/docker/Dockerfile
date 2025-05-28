FROM ghcr.io/netcracker/qubership-backup-daemon:main_alpine

ENV CONSUL_HOME=/opt/consul \
    CONSUL_BACKUP=/opt/consul/backup-storage

COPY backup-daemon.conf /etc/backup-daemon.conf
COPY docker/requirements.txt ${CONSUL_HOME}/requirements.txt
COPY scripts/ ${CONSUL_HOME}/scripts/

RUN set -x \
    && apk add --update --no-cache curl py3-pip apk-tools \
    && pip3 install -r ${CONSUL_HOME}/requirements.txt \
    && rm -rf /var/cache/apk/*

# Upgrade all tools to avoid vulnerabilities
RUN set -x && apk upgrade --no-cache --available

RUN chmod +x ${CONSUL_HOME}/scripts/*.py

RUN chmod -R 777 ${CONSUL_HOME}/scripts

WORKDIR ${CONSUL_HOME}

USER 100:0

VOLUME ["${CONSUL_BACKUP}"]