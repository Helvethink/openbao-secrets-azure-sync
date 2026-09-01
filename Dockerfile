FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/openbao-kv-azure-sync ./cmd/openbao-kv-azure-sync

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
	addgroup -S app && \
	adduser -S -G app app

WORKDIR /app

COPY --from=build /out/openbao-kv-azure-sync /usr/local/bin/openbao-kv-azure-sync
COPY configs/config.docker.yaml /app/config.yaml

USER app
EXPOSE 8080 9090

ENTRYPOINT ["openbao-kv-azure-sync"]
CMD ["-config", "/app/config.yaml"]
