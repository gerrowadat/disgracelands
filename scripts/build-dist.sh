#!/bin/sh
#
# Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
# of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
# Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
# (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
# and DikuMUD licenses; see LICENSE. Non-commercial use only.
#
# Cross-compile dlmud and dlctl for every platform a release ships, and
# package each pair into an archive with the licence and the README.
#
# The platform list below is the whole definition of "what a release
# supports". release.yml runs this in its full-suite job, so the list is
# also the cross-compile check: a change that breaks the build for one of
# these fails the release rather than the download.
#
# Nothing here needs a cross toolchain. CGO_ENABLED=0 across the board --
# which the tree can afford because it has no C dependencies at all (the
# same property that lets build/Dockerfile use distroless/static, and the
# reason the pluggable formats are a compiled-in registry rather than Go
# plugins) -- so the host's own Go toolchain produces every one of these
# natively, in seconds, with no emulation and no apt.
#
# Usage: scripts/build-dist.sh [outdir]        # default: dist
# Environment: VERSION, COMMIT, DATE override what git and date report.

set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

# zip writes DOS timestamps, which are local time with no zone recorded,
# so the same staged tree archived in two zones produces two different
# files. Pinning the zone here is the other half of pinning the mtimes
# below.
TZ=UTC
export TZ

OUT=${1:-dist}

# The same three variables `make build` stamps a local binary with, and
# the same defaults for the first two. release.yml passes the version the
# suite agreed on, and the commit it is releasing.
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo devel)}
COMMIT=${COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}

# DATE is where this parts company with `make build`, which uses the wall
# clock. The commit's own date instead, so that everything stamped into
# these binaries is a function of the commit and nothing else -- run this
# twice an hour apart on the same tag and the archives are byte for byte
# identical, which is the whole point of pinning the mtimes, modes and
# member order further down. A build date that is the minute the runner
# happened to start would undo all of it from inside the binary.
DATE=${DATE:-$(git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ 2>/dev/null ||
	date -u +%Y-%m-%dT%H:%M:%SZ)}

BUILDINFO=github.com/gerrowadat/disgracelands/internal/buildinfo

# 64-bit only, on purpose. linux/amd64 and linux/arm64 are what the
# container image already publishes; windows/amd64 is the one platform
# here with no image to fall back on, which is most of why it is worth
# shipping a binary for. Everything else the toolchain can target --
# 386, armv7, darwin -- builds today and is simply not published; add a
# line here and it is, at the cost of two more archives per release.
PLATFORMS="linux/amd64 linux/arm64 windows/amd64"

# What travels with the binaries. LICENSE is not optional: the CircleMUD
# and DikuMUD licences have to ship with any copy of the server, and an
# archive somebody downloads is a copy, exactly as the container image is
# (scripts/license-check.sh checks for both).
EXTRAS="LICENSE README.md BUILDING.md"

case "$PLATFORMS" in
*windows*)
	command -v zip >/dev/null 2>&1 || {
		echo "build-dist.sh: zip is needed for the windows archives" >&2
		exit 1
	}
	;;
esac

rm -rf "$OUT"
mkdir -p "$OUT"

echo "==> Building $VERSION ($(echo "$COMMIT" | cut -c1-7)) for: $PLATFORMS"

archives=""

for platform in $PLATFORMS; do
	os=${platform%/*}
	arch=${platform#*/}
	name="disgracelands-$VERSION-$os-$arch"
	stage="$OUT/$name"
	ext=""
	if [ "$os" = "windows" ]; then
		ext=".exe"
	fi

	# vet before build, because `go build` below compiles only what the
	# two binaries import -- not the tests. A _test.go naming a
	# Unix-only symbol builds clean and fails `GOOS=windows go vet`, and
	# that is not hypothetical: internal/signals broke both halves at
	# once the first time this script ran for real, the package in
	# Name() and its test file in syscall.Kill. vet type-checks tests,
	# so it is the stricter of the two and costs seconds.
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go vet ./...

	mkdir -p "$stage"
	for bin in dlmud dlctl; do
		CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
			go build -trimpath \
			-ldflags="-s -w \
				-X $BUILDINFO.version=$VERSION \
				-X $BUILDINFO.commit=$COMMIT \
				-X $BUILDINFO.date=$DATE" \
			-o "$stage/$bin$ext" "./cmd/$bin"
	done
	# Deliberately unquoted: $EXTRAS is a list of paths, not one path.
	# shellcheck disable=SC2086
	cp $EXTRAS "$stage/"

	# Both archive formats record permissions, and both `go build` and
	# `cp` produce theirs by masking against the builder's umask -- so
	# the same tree packaged on a umask 002 machine and a umask 022 one
	# differs in bytes while being identical in content. Set them rather
	# than inherit them.
	find "$stage" -type f -exec chmod 0644 {} +
	chmod 0755 "$stage" "$stage/dlmud$ext" "$stage/dlctl$ext"

	# One fixed mtime across everything staged, so the archives are byte
	# for byte reproducible from the same commit rather than carrying the
	# minute they happened to be built in. -trimpath and the pinned
	# ldflags already do that for the binaries themselves; without this
	# the container around them would undo it, and a SHA256SUMS nobody
	# can reproduce is a checksum of trust in the runner rather than in
	# the bytes.
	#
	# 1980-01-01, not the epoch: a zip's DOS timestamp cannot represent
	# anything earlier, so zip would silently clamp 1970 to 1980 and the
	# tar and the zip would disagree about the same file.
	find "$stage" -exec touch -h -d "@315532800" {} + 2>/dev/null ||
		find "$stage" -exec touch -h -t 198001010000.00 {} +

	case "$os" in
	windows)
		# -X drops the extra fields (uid/gid, high-precision times) that
		# would otherwise vary per run and per machine, and the sorted
		# `find` feeding `zip -@` replaces `zip -r`, which walks the
		# directory in readdir order -- an order that is the filesystem's
		# to choose and not the same everywhere. tar gets the same
		# guarantee from --sort=name below.
		(cd "$OUT" && find "$name" | LC_ALL=C sort | zip -qX "$name.zip" -@)
		archives="$archives $name.zip"
		;;
	*)
		# --sort and the ownership overrides for the same reason; gzip
		# -n so the header carries no timestamp or original filename.
		tar --sort=name --owner=0 --group=0 --numeric-owner \
			-C "$OUT" -cf - "$name" | gzip -9n > "$OUT/$name.tar.gz"
		archives="$archives $name.tar.gz"
		;;
	esac
	rm -rf "$stage"
done

# One checksum file for the lot, in the format `sha256sum -c` reads, so a
# download can be verified with `sha256sum -c SHA256SUMS` from inside the
# directory it was downloaded to. Named one by one from the loop rather
# than globbed: a PLATFORMS list with no Windows entry in it would leave
# `*.zip` unmatched, and the shell would hand sha256sum the pattern.
#
# shellcheck disable=SC2086
(cd "$OUT" && sha256sum $archives > SHA256SUMS)

echo "==> $OUT:"
ls -l "$OUT"
