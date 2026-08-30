# Multi-stage build for the Go server. See docs/design/go-port-plan.md §9.3.
#
# Build from the repository root:
#   docker build -f build/Dockerfile -t disgracelands .
#
# The runtime image is distroless/static with no shell. That is deliberate:
# the C server's answer to "keep it running" was the autorun script looping in
# a shell, and this replaces that with the container runtime's restart policy
# plus proper SIGTERM handling in the server itself.

# --platform=$BUILDPLATFORM pins the *builder* stage to the machine doing the
# building, and GOOS/GOARCH below come from the target instead. Without it, a
# multi-architecture build (release.yml pushes linux/amd64 and linux/arm64)
# would run an emulated arm64 toolchain under QEMU to produce the arm64 image,
# which is minutes of emulated compilation for a binary Go will cross-compile
# natively in seconds. CGO_ENABLED=0 is what makes that legitimate rather than
# a shortcut: there is no C toolchain in play to have to match.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

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

# Set by buildx from the platform being built for. Empty under the legacy
# (non-BuildKit) builder, and an empty GOOS/GOARCH is the same as an unset one
# to the go command, so a plain `docker build` still builds for the host.
ARG TARGETOS
ARG TARGETARCH

# CGO_ENABLED=0 for a static binary: it is what lets the runtime image be
# distroless/static, and it is also why the pluggable formats are a compiled-in
# registry rather than Go plugins (see docs/design/go-port-plan.md §5.1).
ENV CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH}
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
# server, including container images (docs/design/go-port-plan.md §12, requirement 1).
COPY --from=build /src/LICENSE /LICENSE

COPY --from=build /out/dlmud /out/dlctl /usr/local/bin/

# data/ is mutable state - players, houses, boards, mail, and world files
# edited in-game - so it is a volume rather than image content.
VOLUME ["/data"]

USER nonroot:nonroot
EXPOSE 4443 8080

ENTRYPOINT ["/usr/local/bin/dlmud"]
CMD ["--lib-dir=/data"]
