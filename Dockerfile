FROM golang:1.26.0-alpine3.22 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/labreserve ./cmd/labreserve

FROM alpine:3.22.1
COPY --from=build --chown=65532:65532 /out/labreserve /labreserve
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/labreserve"]
