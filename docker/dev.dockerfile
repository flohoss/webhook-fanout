ARG V_GOLANG
FROM golang:${V_GOLANG}-alpine AS final
WORKDIR /src

ARG V_AIR
RUN go install github.com/air-verse/air@v${V_AIR}

COPY ./go.mod ./go.sum ./
RUN go mod download
