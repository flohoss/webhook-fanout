ARG V_GOLANG
FROM golang:${V_GOLANG}-alpine AS build

WORKDIR /src
COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /src/webhook-fanout .

FROM scratch AS final

ARG APP_VERSION
ARG BUILD_TIME
ARG REPO_URL

COPY --from=build --chown=65532:65532 /src/webhook-fanout /src/webhook-fanout

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/src/webhook-fanout"]
