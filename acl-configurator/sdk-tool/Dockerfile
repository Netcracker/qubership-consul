FROM quay.io/operator-framework/operator-sdk:v1.16@sha256:b81e0c0151faf897a27c42c4c859b2e20b0c1550480d9bbfae71c0875fd26797

RUN microdnf install --nodocs \
    gcc \
    git \
    && microdnf clean all

USER 1000

ENTRYPOINT ["/bin/bash"]