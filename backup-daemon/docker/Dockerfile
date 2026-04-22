FROM ghcr.io/netcracker/qubership-backup-daemon:main_alpine

ENV CONSUL_HOME=/opt/consul \
    CONSUL_BACKUP=/opt/consul/backup-storage

COPY backup-daemon.conf /etc/backup-daemon.conf
COPY docker/requirements.txt ${CONSUL_HOME}/requirements.txt
COPY scripts/ ${CONSUL_HOME}/scripts/

RUN echo "https://dl-cdn.alpinelinux.org/alpine/v3.23/main" > /etc/apk/repositories && \
    echo "https://dl-cdn.alpinelinux.org/alpine/v3.23/community" >> /etc/apk/repositories

RUN set -x \
    && apk add --update --no-cache curl py3-pip apk-tools \
    && pip3 install -r ${CONSUL_HOME}/requirements.txt \
    && rm -rf /var/cache/apk/*

#https://peps.python.org/pep-0668/
RUN apk add --no-cache wget python3 py3-pip \
    && pip install --break-system-packages --upgrade pip==26.0.1 setuptools==80.10.2 wheel==0.46.2 jaraco.context==6.1.0

# Upgrade all tools to avoid vulnerabilities
RUN set -x && apk upgrade --no-cache --available

# Upgrade tools from edge to avoid vulnerabilities
RUN echo "https://dl-cdn.alpinelinux.org/alpine/edge/main" > /etc/apk/repositories  \
    && echo "https://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories \
    && apk add --update --upgrade --no-cache \
      py3-setuptools py3-setuptools-pyc musl-utils musl libssl3 libcrypto3 nghttp2-libs libuuid libexpat zlib libcurl curl

RUN chmod +x ${CONSUL_HOME}/scripts/*.py

RUN chmod -R 777 ${CONSUL_HOME}/scripts

WORKDIR ${CONSUL_HOME}

USER 100:0

VOLUME ["${CONSUL_BACKUP}"]
