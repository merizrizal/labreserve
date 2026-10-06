FROM golang:1.26.0-alpine3.22 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/labreserve ./cmd/labreserve

FROM build AS verify-build
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/labreserve-e2e ./tests/e2e/support

FROM alpine:3.22.1 AS verify-app
COPY --from=verify-build --chown=65532:65532 /out/labreserve-e2e /labreserve-e2e
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/labreserve-e2e"]

FROM alpine:3.22.1
COPY --from=build --chown=65532:65532 /out/labreserve /labreserve
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/labreserve"]
