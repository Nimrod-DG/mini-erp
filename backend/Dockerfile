# The API image. Two stages: build with the toolchain, ship without it.
#
# Phase 8 builds this in CI; Phase 9 is what deploys it. Nothing here is
# deployment-specific on purpose — the image reads its whole configuration from
# the environment, so the same artefact runs locally and in Cloud Run.

FROM golang:1.25-alpine AS build

WORKDIR /src

# Dependencies first, in their own layer: go.mod and go.sum change far less often
# than the code does, so a source edit does not re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 for a static binary, which is what lets the final stage be a
# scratch-like image with no libc. -trimpath keeps build paths out of the binary,
# and the ldflags drop the symbol table and DWARF — neither is useful in a
# container you cannot attach a debugger to anyway.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# The migration runner ships too. Migrations are a separate process, never the
# API's job (see internal/db) — but they have to run *somewhere*, and a release
# that cannot migrate itself is a release that needs a laptop.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# distroless/static rather than alpine: no shell, no package manager, and nothing
# to `apk add` if this container is ever compromised. `nonroot` is uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot

# The migrations are read from an embed.FS inside the binary, so there is nothing
# to copy alongside it — which is also why `migrate` cannot drift from the schema
# the binary expects.
COPY --from=build /out/api /api
COPY --from=build /out/migrate /migrate

USER nonroot:nonroot

# I7: timestamps are TIMESTAMPTZ stored UTC, and Group J asserts the process
# agrees. distroless has no tzdata, so this is the only zone available anyway —
# stated rather than relied upon.
ENV TZ=UTC

EXPOSE 8080

ENTRYPOINT ["/api"]
