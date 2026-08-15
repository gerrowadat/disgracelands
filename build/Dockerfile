# Multi-stage build for the Go server. See docs/go-port-plan.md §9.3.
#
# Build from the repository root:
#   docker build -f build/Dockerfile -t disgracelands .
#
# The runtime image is distroless/static with no shell. That is deliberate:
# the C server's answer to "keep it running" was the autorun script looping in
# a shell, and this replaces that with the container runtime's restart policy
# plus proper SIGTERM handling in the server itself.

FROM golang:1.25-alpine AS build

WORKDIR /src

# Dependencies first, so a source-only change does not re-download them.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
COPY LICENSE ./

ARG VERSION=devel
ARG COMMIT=unknown
ARG DATE=unknown

# CGO_ENABLED=0 for a static binary: it is what lets the runtime image be
# distroless/static, and it is also why the pluggable formats are a compiled-in
# registry rather than Go plugins (see docs/go-port-plan.md §5.1).
ENV CGO_ENABLED=0
RUN go build -trimpath \
      -ldflags="-s -w \
        -X github.com/gerrowadat/disgracelands/internal/buildinfo.version=${VERSION} \
        -X github.com/gerrowadat/disgracelands/internal/buildinfo.commit=${COMMIT} \
        -X github.com/gerrowadat/disgracelands/internal/buildinfo.date=${DATE}" \
      -o /out/dlmud ./cmd/dlmud \
 && go build -trimpath \
      -ldflags="-s -w \
        -X github.com/gerrowadat/disgracelands/internal/buildinfo.version=${VERSION} \
        -X github.com/gerrowadat/disgracelands/internal/buildinfo.commit=${COMMIT} \
        -X github.com/gerrowadat/disgracelands/internal/buildinfo.date=${DATE}" \
      -o /out/dlctl ./cmd/dlctl


FROM gcr.io/distroless/static-debian12:nonroot

# The CircleMUD and DikuMUD licenses travel with any distribution of the
# server, including container images (docs/go-port-plan.md §12, requirement 1).
COPY --from=build /src/LICENSE /LICENSE

COPY --from=build /out/dlmud /out/dlctl /usr/local/bin/

# lib/ is mutable state - players, houses, boards, mail, and OLC-edited world
# files - so it is a volume rather than image content.
VOLUME ["/lib"]

USER nonroot:nonroot
EXPOSE 4443 8080

ENTRYPOINT ["/usr/local/bin/dlmud"]
CMD ["--lib-dir=/lib"]
