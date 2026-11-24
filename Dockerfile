# Versions are pinned so that a rebuild nobody asked for cannot change the
# toolchain under a running ledger. The Go version is go.mod's.

# The interface, built for real use: it posts every method to /api on its own
# origin and holds no stand-in data. It reads the contract's manifest from
# internal/contract, so that file keeps its place relative to web/.
FROM node:24.18.0-alpine3.24 AS interface
WORKDIR /src
COPY web/package.json web/package-lock.json web/
RUN npm --prefix web ci
COPY web web
COPY internal/contract/contract.json internal/contract/contract.json
RUN npm --prefix web run build

# The program, with the interface embedded. The SQLite driver is pure Go, so the
# binary needs no C library and runs on any base.
FROM golang:1.26.6-alpine3.24 AS program
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
COPY --from=interface /src/web/dist/ internal/webui/dist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/finwise ./cmd/finwise

# Alpine and not an empty image: the scheduled backup runs `finwise backup`
# from a shell, with a dated file name, and prunes old copies. The program does
# not write to its root: its two files live where the deployment mounts them.
FROM alpine:3.24
RUN addgroup -g 1000 finwise && adduser -D -H -u 1000 -G finwise -s /sbin/nologin finwise
COPY --from=program /out/finwise /usr/local/bin/finwise
USER 1000:1000
# FINWISE_LISTEN defaults to 127.0.0.1:8080, which nothing outside the
# container can reach: a container sets it to 0.0.0.0:8080.
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/finwise"]
CMD ["serve"]
