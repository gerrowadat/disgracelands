# Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
# of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
# Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
# (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
# and DikuMUD licenses; see LICENSE. Non-commercial use only.
#
# Developer conveniences for the Go tree. Nothing here is required to build
# the server -- `go build ./...` and `go test ./...` remain the truth, and CI
# calls the same underlying commands rather than this file. What this is for
# is the half-dozen invocations you would otherwise keep in shell history:
# start a server on a tiny world, start one on a throwaway data directory,
# point one at a real one, connect to it.
#
# `make` on its own lists the targets. See docs/developer.md for the
# workflows they belong to.
#
# The C server in reference/ has its own build and is not driven from here;
# see reference/moderncserver/README.md.

GO      ?= go
PKG     ?= ./...
OUT     ?= out

# Where the server reads its data. `make run LIB=/srv/disgracelands/lib` runs
# against a real directory; the default is the one in the repository.
LIB     ?= examples/stock/binary

# Listen addresses for the dev targets. The plaintext port is the C server's
# habitual 4000; 4443 is the TLS one; 9090 carries /metrics, /healthz, /readyz.
HOST         ?= 127.0.0.1
PORT         ?= 4000
TLS_PORT     ?= 4443
METRICS_PORT ?= 9090

LOG_LEVEL ?= debug

# Extra flags for any run target: `make run FLAGS="--restrict --no-specials"`.
FLAGS ?=

# The scratch directory is deleted and recreated by `make run-fresh`, so it is
# deliberately not overridable: nothing outside out/ is ever removed.
SCRATCH := $(OUT)/scratch-lib
DEVCERT := $(OUT)/dev

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo devel)
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

BUILDINFO := github.com/gerrowadat/disgracelands/internal/buildinfo
LDFLAGS   := -X $(BUILDINFO).version=$(VERSION) \
             -X $(BUILDINFO).commit=$(COMMIT) \
             -X $(BUILDINFO).date=$(DATE)

# Plaintext telnet, no certificate, diagnostics on. The TLS listener is on by
# default and would demand a certificate, so every dev target turns it off
# explicitly rather than leaving you to decode the startup error.
RUN_FLAGS = --lib-dir=$(LIB) \
            --listen-telnet=$(HOST):$(PORT) \
            --listen-telnets= \
            --metrics-addr=$(HOST):$(METRICS_PORT) \
            --log-level=$(LOG_LEVEL) \
            $(FLAGS)

.DEFAULT_GOAL := help

##@ Getting started

.PHONY: help
help: ## List the targets
	@awk 'BEGIN {FS = ":.*##"} \
	     /^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next} \
	     /^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' \
	     $(MAKEFILE_LIST)
	@printf '\nVariables: LIB=%s PORT=%s TLS_PORT=%s METRICS_PORT=%s LOG_LEVEL=%s FLAGS=\n\n' \
	     '$(LIB)' '$(PORT)' '$(TLS_PORT)' '$(METRICS_PORT)' '$(LOG_LEVEL)'

##@ Running a server

.PHONY: run
run: ## Run against the real data directory (LIB), plaintext telnet
	$(GO) run ./cmd/dlmud $(RUN_FLAGS)

.PHONY: run-mini
run-mini: ## Run against the tiny world (--mini-mud): three zones, boots instantly
	$(GO) run ./cmd/dlmud $(RUN_FLAGS) --mini-mud

# A fresh directory has no roster, and the first character created on an empty
# roster is made an Implementor (internal/game/create.go, from db.c's "if this
# is our first player --- he be God"). That is the quickest route to a level
# 34 character, and it is why this target exists rather than being a variant
# of `run`: on LIB itself you would only get one, once.
.PHONY: run-fresh
run-fresh: $(SCRATCH) ## Run on a throwaway copy of the data, no players (first login is an Implementor)
	$(GO) run ./cmd/dlmud $(RUN_FLAGS) --lib-dir=$(SCRATCH)

.PHONY: run-tls
run-tls: $(DEVCERT)/cert.pem ## Run with the TLS listener, using a self-signed dev certificate
	$(GO) run ./cmd/dlmud \
	  --lib-dir=$(LIB) \
	  --listen-telnets=$(HOST):$(TLS_PORT) \
	  --tls-cert=$(DEVCERT)/cert.pem --tls-key=$(DEVCERT)/key.pem \
	  --metrics-addr=$(HOST):$(METRICS_PORT) \
	  --log-level=$(LOG_LEVEL) $(FLAGS)

.PHONY: connect
connect: ## Connect to a running dev server (telnet, falls back to nc)
	@command -v telnet >/dev/null 2>&1 && exec telnet $(HOST) $(PORT) || exec nc $(HOST) $(PORT)

.PHONY: connect-tls
connect-tls: ## Connect to the TLS listener, accepting the self-signed certificate
	@openssl s_client -quiet -verify_return_error -CAfile $(DEVCERT)/cert.pem \
	  -connect $(HOST):$(TLS_PORT)

.PHONY: health
health: ## Print /healthz, /readyz and the server's own metrics from a running server
	@curl -sS -o /dev/null -w 'healthz %{http_code}\n' http://$(HOST):$(METRICS_PORT)/healthz
	@curl -sS -o /dev/null -w 'readyz  %{http_code}\n' http://$(HOST):$(METRICS_PORT)/readyz
	@curl -sS http://$(HOST):$(METRICS_PORT)/metrics | grep '^dlmud_' || true

# cp -a then delete the player state: a fresh directory is the point, and
# $(LIB)/pfiles may hold real ex-players' hashes and mail if LIB has been
# pointed at a converted archive, which have no business being copied around.
# The empty directories are recreated because a missing one is a first-login
# failure rather than an empty roster.
$(SCRATCH):
	@echo "==> Building a scratch data directory in $(SCRATCH) from $(LIB)"
	@rm -rf $(SCRATCH)
	@mkdir -p $(dir $(SCRATCH))
	@cp -a $(LIB)/. $(SCRATCH)/
	@rm -rf $(SCRATCH)/pfiles $(SCRATCH)/plrobjs $(SCRATCH)/plralias $(SCRATCH)/house \
	        $(SCRATCH)/etc/players $(SCRATCH)/etc/plrmail
	@mkdir -p $(SCRATCH)/pfiles $(SCRATCH)/plrobjs $(SCRATCH)/plralias $(SCRATCH)/house

.PHONY: scratch
scratch: $(SCRATCH) ## Rebuild the throwaway data directory used by run-fresh

# A local certificate for exercising the TLS path -- which is the default
# listener in production, and so the one worth testing before shipping. Not
# trusted by anything, and it never leaves out/.
$(DEVCERT)/cert.pem:
	@command -v openssl >/dev/null 2>&1 || { echo "openssl is needed to make a dev certificate"; exit 1; }
	@echo "==> Generating a self-signed certificate for localhost in $(DEVCERT)"
	@mkdir -p $(DEVCERT)
	@openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
	  -subj '/CN=localhost' \
	  -addext 'subjectAltName=DNS:localhost,IP:127.0.0.1' \
	  -keyout $(DEVCERT)/key.pem -out $(DEVCERT)/cert.pem 2>/dev/null

.PHONY: dev-cert
dev-cert: $(DEVCERT)/cert.pem ## Generate the self-signed localhost certificate for run-tls

##@ Building and checking

.PHONY: build
build: ## Build both binaries into out/, version-stamped
	$(GO) build -trimpath -ldflags='$(LDFLAGS)' -o $(OUT)/dlmud ./cmd/dlmud
	$(GO) build -trimpath -ldflags='$(LDFLAGS)' -o $(OUT)/dlctl ./cmd/dlctl

.PHONY: test
test: ## go test -race (the race detector is not optional here; see the plan's §3.1)
	$(GO) test -race -count=1 $(PKG)

.PHONY: test-fast
test-fast: ## go test without -race, for a quick inner loop
	$(GO) test $(PKG)

.PHONY: cover
cover: ## Run the tests with coverage and open the HTML report
	$(GO) test -coverprofile=$(OUT)/coverage.out $(PKG)
	$(GO) tool cover -html=$(OUT)/coverage.out

.PHONY: fmt
fmt: ## gofmt -w the tree
	gofmt -l -w .

.PHONY: vet
vet: ## go vet
	$(GO) vet $(PKG)

.PHONY: lint
lint: ## golangci-lint, fetching the version CI uses if it is not installed
	@if command -v golangci-lint >/dev/null 2>&1; then \
	  golangci-lint run; \
	else \
	  echo "golangci-lint not on PATH; running $(GOLANGCI_VERSION) via go run"; \
	  $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run; \
	fi

.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy

# What CI does, minus the parts that need a C toolchain (see `make parity`) or
# a 32-bit one. Green here is not a promise that CI is green, but red here
# certainly means it is not.
.PHONY: check
# The version CI pins. `make lint` fetches it rather than skipping: a lint
# that silently does not run looks exactly like a lint that passed, which is
# how a clean `make check` and a red CI happen at the same time.
GOLANGCI_VERSION ?= v2.12.2

check: ## Build, vet, format-check, lint, test, license and world-lint -- the full local check `make release` runs before tagging
	@unformatted=$$(gofmt -l .); \
	  if [ -n "$$unformatted" ]; then echo "These files need gofmt:"; echo "$$unformatted"; exit 1; fi
	$(GO) build $(PKG)
	$(MAKE) vet lint test world-lint license

.PHONY: parity
parity: ## Check the Go and C world loaders agree (builds the C server; slow)
	./scripts/world-parity.sh

.PHONY: license
license: ## Check the CircleMUD/DikuMUD license obligations
	./scripts/license-check.sh

.PHONY: release
release: ## Cut a release: make release BUMP=patch|minor|major (or BUMP=v1.2.3)
	@test -n "$(BUMP)" || { echo "usage: make release BUMP=patch|minor|major|v1.2.3"; exit 2; }
	./scripts/release.sh $(BUMP)

# `make check` is an approximation of CI assembled by hand, and an
# approximation drifts: it does not run the actions, the 32-bit steps or the
# container build, and it cannot notice a workflow change at all. These
# targets run .github/workflows/go.yml itself, in containers, via act. They
# need Docker. See docs/developer.md for what they do and do not reproduce.
ACT_VERSION ?= v0.2.89

# Same reasoning as GOLANGCI_VERSION above: fetch the pinned version rather
# than skip. `act` is a single Go binary, so `go run` is a fine way to get it.
ACT_BIN = $(shell command -v act 2>/dev/null || echo "$(GO) run github.com/nektos/act@$(ACT_VERSION)")

# This repo is routinely developed from git worktrees, whose own `.git` is a
# *file* ("gitdir: /path/to/real/.git/worktrees/name"), not a directory.
# act's checkout copies the working tree into the job container, but that
# pointer's target is a host path the container has no other way to see --
# so any step that runs git (the "Verify go.mod is tidy" step's `git diff`)
# fails with "fatal: not a git repository: (null)" the moment `make ci` is
# run from anywhere but the primary checkout. --git-common-dir resolves the
# indirection to the one real .git regardless, so bind-mounting *that* into
# the container at the same path fixes it -- and is a harmless mount of a
# directory onto itself for a plain clone, where --git-common-dir is just
# $(CURDIR)/.git.
GIT_COMMON_DIR := $(shell git rev-parse --git-common-dir 2>/dev/null)
ACT = $(ACT_BIN) --container-options "-v $(GIT_COMMON_DIR):$(GIT_COMMON_DIR):ro"

# Scoped to go.yml, the day-to-day workflow (build/vet/lint/test on every
# push and pull request). release.yml -- the full regression suite --
# runs the 32-bit toolchain, a C build and a container build
# unconditionally; act can run it too (`make ci-job JOB=full-suite
# WORKFLOW=.github/workflows/release.yml`), but it is slow enough, and
# close enough to release.sh's own local pre-flight, that it is rarely
# worth reaching for over just pushing a test tag.
CI_WORKFLOW ?= .github/workflows/go.yml

.PHONY: ci
ci: ## Run go.yml locally, in containers (needs Docker; slow)
	$(ACT) -W $(CI_WORKFLOW)

.PHONY: ci-job
ci-job: ## Run one job: make ci-job JOB=test (test|lint; add WORKFLOW=... for release.yml's jobs)
	@test -n "$(JOB)" || { echo "usage: make ci-job JOB=<name>; try 'make ci-list'"; exit 1; }
	$(ACT) -W $(CI_WORKFLOW) -j $(JOB)

.PHONY: ci-list
ci-list: ## List the jobs `make ci` would run
	$(ACT) -W $(CI_WORKFLOW) --list

# .actrc passes --reuse, so the job containers survive between runs and carry
# their caches with them. That is the difference between a second run taking
# seconds and taking minutes, and it is also how a run goes stale.
#
# The volumes matter more than the containers, and this is the trap: act keeps
# each job's working directory in a named volume that outlives the container,
# and it populates that directory with `docker cp`, which overwrites files but
# never removes them. So a file you deleted on your branch stays in the volume
# and keeps getting compiled. That reads as a failure in code you cannot find
# -- `undefined: New` in a _test.go that is not on disk -- and it could as
# easily read as a pass. Removing the containers alone does not fix it.
.PHONY: ci-clean
ci-clean: ## Remove the containers *and volumes* act reuses between runs
	@ids=$$(docker ps -aq --filter "name=act-"); \
	  if [ -n "$$ids" ]; then docker rm -f $$ids; else echo "no act containers"; fi
	@vols=$$(docker volume ls -q --filter "name=act-"); \
	  if [ -n "$$vols" ]; then docker volume rm -f $$vols; else echo "no act volumes"; fi

##@ Data and tooling

.PHONY: world-lint
world-lint: ## Lint the world files under LIB
	$(GO) run ./cmd/dlctl world lint --world-dir=$(LIB)/world

.PHONY: world-dump
world-dump: ## Dump the loaded world as canonical JSON to out/world.json
	@mkdir -p $(OUT)
	$(GO) run ./cmd/dlctl world dump --world-dir=$(LIB)/world --out=$(OUT)/world.json
	@echo "==> $(OUT)/world.json"

# The one-off that turns an archived CircleMUD lib/ into something this server
# will run on: player database reformatted, text transcoded to UTF-8. Convert
# once, then `make run LIB=$(TO)`.
.PHONY: convert
convert: ## Convert an original data directory: make convert FROM=/path/to/lib TO=out/converted
	@test -n "$(FROM)" && test -n "$(TO)" || { echo 'usage: make convert FROM=/path/to/lib TO=out/converted'; exit 2; }
	$(GO) run ./cmd/dlctl convert --from=$(FROM) --to=$(TO)

# The yaml equivalent: every subsystem, one lib/ to one fresh yaml
# directory, in one command. Point it at the original archive, not at
# $(TO) above — the two do not chain, see docs/operations.md. Then
# `make run LIB=$(TO) FLAGS="--world-format=yaml --state-format=yaml --names-format=yaml --messages-format=yaml --socials-format=yaml --help-format=yaml"`.
.PHONY: lib-import
lib-import: ## Convert an original data directory into yaml: make lib-import FROM=/path/to/lib TO=out/yaml
	@test -n "$(FROM)" && test -n "$(TO)" || { echo 'usage: make lib-import FROM=/path/to/lib TO=out/yaml'; exit 2; }
	$(GO) run ./cmd/dlctl lib import --from-dir=$(FROM) --to-dir=$(TO)

.PHONY: roster
roster: ## List the characters in the player directory under LIB
	$(GO) run ./cmd/dlctl pfile dump --player-dir=$(LIB)/pfiles

.PHONY: ctl
ctl: ## Run dlctl: make ctl ARGS="pfile dump --name=Someone"
	$(GO) run ./cmd/dlctl $(ARGS)

##@ Containers

.PHONY: docker
docker: ## Build the container image as disgracelands:dev
	docker build -f build/Dockerfile \
	  --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) \
	  -t disgracelands:dev .

.PHONY: compose-up
compose-up: ## Bring up the local compose stack (telnet :4000, metrics :9090)
	docker compose -f build/docker-compose.yml up --build

.PHONY: compose-down
compose-down: ## Stop the local compose stack
	docker compose -f build/docker-compose.yml down

##@ Housekeeping

.PHONY: clean
clean: ## Remove out/: binaries, scratch data directory, dev certificate
	rm -rf $(OUT)
