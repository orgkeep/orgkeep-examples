# Built from the sources, no Go needed on the machine:
#   docker build -t ghcr.io/orgkeep/gateway-go .
# (the demo compose builds it with docker compose up --build)

FROM golang:1.25-alpine AS build
WORKDIR /build
# the dependencies first, in their own layer: downloaded again only when go.mod changes
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
# a static binary: the final image has no C library
RUN CGO_ENABLED=0 go build -o gateway .

# nothing but the binary and the CA certificates, as a non-root user
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /build/gateway /gateway
EXPOSE 8292
ENTRYPOINT ["/gateway"]
