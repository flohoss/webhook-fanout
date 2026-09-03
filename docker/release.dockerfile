ARG V_GOLANG
FROM golang:${V_GOLANG}-alpine AS build

WORKDIR /src
COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /src/webhook-fanout .

FROM scratch AS final

ARG APP_VERSION=dev
ARG BUILD_TIME=unknown
ARG REPO_URL=

ENV APP_VERSION=${APP_VERSION} \
    BUILD_TIME=${BUILD_TIME} \
    REPO_URL=${REPO_URL}

COPY --from=build --chown=65532:65532 /src/webhook-fanout /src/webhook-fanout

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/src/webhook-fanout"]
