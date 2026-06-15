FROM quay.io/operator-framework/operator-sdk:v1.16

RUN microdnf install --nodocs \
    gcc \
    git \
    && microdnf clean all

USER 1000

ENTRYPOINT ["/bin/bash"]