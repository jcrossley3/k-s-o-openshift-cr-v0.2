FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/k-s-o-openshift \
    USER_UID=1001 \
    USER_NAME=k-s-o-openshift

# install operator binary
COPY build/_output/bin/k-s-o-openshift ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
