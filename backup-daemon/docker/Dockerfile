FROM ghcr.io/netcracker/qubership-backup-daemon-go:0.1.19@sha256:33653b0cf2b13a7df9daeac4a5186c4b3daeb9d80680166a316db46547333508

ENV CONSUL_HOME=/opt/consul \
    CONSUL_BACKUP=/opt/consul/backup-storage

RUN echo "https://dl-cdn.alpinelinux.org/alpine/v3.24/main" > /etc/apk/repositories && \
    echo "https://dl-cdn.alpinelinux.org/alpine/v3.24/community" >> /etc/apk/repositories

COPY backup-daemon.conf /etc/backup-daemon.conf
COPY docker/requirements.txt ${CONSUL_HOME}/requirements.txt
COPY scripts/ ${CONSUL_HOME}/scripts/

RUN set -x \
    && apk add --update --no-cache \
      python3 \
      py3-requests \
      py3-pip \
    && if pip3 install --help | grep -q -- '--break-system-packages'; then PIP_BREAK_FLAG='--break-system-packages'; else PIP_BREAK_FLAG=''; fi \
    && pip3 install --no-cache-dir ${PIP_BREAK_FLAG} setuptools \
    && pip3 install --no-cache-dir ${PIP_BREAK_FLAG} -r ${CONSUL_HOME}/requirements.txt \
    && rm -rf /var/cache/apk/*

# Upgrade all tools to avoid vulnerabilities
RUN set -x && apk upgrade --no-cache --available

RUN chmod +x ${CONSUL_HOME}/scripts/*.py

USER 100:0
VOLUME ["${CONSUL_BACKUP}"]
