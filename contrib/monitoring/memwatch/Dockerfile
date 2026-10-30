# Build from the repository root:
#   docker build -f contrib/monitoring/memwatch/Dockerfile -t memwatch:local .
FROM golang:1.27 AS build
WORKDIR /src
# memwatch only needs the module files and its own package; it imports no Dgraph code.
COPY go.mod go.sum ./
COPY contrib/monitoring/memwatch ./contrib/monitoring/memwatch
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /memwatch ./contrib/monitoring/memwatch

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /memwatch /memwatch
EXPOSE 9095
ENTRYPOINT ["/memwatch"]
