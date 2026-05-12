# check=skip=InvalidDefaultArgInFrom

# minio tag to build/extend, e.g., RELEASE.2024-01-29T03-56-32Z
ARG ELM_RELEASE

FROM amd64/golang:1.25.7
ARG ELM_RELEASE

RUN apt-get update
RUN apt-get install -y \
	jq

RUN mkdir -p -m 0700 ~/.ssh && ssh-keyscan github.com >> ~/.ssh/known_hosts

WORKDIR /build/

COPY ./minio.build.sh ./
COPY ./elm-patch/ ./elm-patch/
COPY ./src/ ./src/

RUN --mount=type=ssh,uid=1000 ssh-add -l
RUN --mount=type=ssh,uid=1000 /bin/bash -x ./minio.build.sh "${ELM_RELEASE}"

FROM redhat/ubi9-minimal

RUN microdnf install -y \
	ca-certificates \
	openssl \
	tzdata \
	&& microdnf clean all

ARG ELM_RELEASE

COPY --from=0 /build/src/minio/minio /usr/bin/minio

COPY docker-entrypoint.sh /usr/bin/docker-entrypoint.sh

ENV PATH="/usr/bin"
ENTRYPOINT ["/usr/bin/docker-entrypoint.sh"]
CMD ["/usr/bin/minio"]
