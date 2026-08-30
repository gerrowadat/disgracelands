// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	_ "github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
	"github.com/gerrowadat/disgracelands/internal/rng"
)

// What the game loop costs, on the real world rather than the test one.
//
// These do not run in CI and are not meant to (`go.yml` is correctness and
// lint only, and benchmark timings on a shared runner are too noisy to gate
// anything). They are `make bench`: the thing to run when you are about to
// change something on the pulse, or when a server is missing its
// checkpoints and you want to know which part.
//
// They exist because writing them once, ad hoc, is what found #322 — where
// `mobileCount`'s full-world scan was 97% of a 3.5ms zone reset and
// indexing it was 451x. Nothing about that was visible from reading the
// code, and it was invisible to every other test in the tree because
// internal/server's own harness builds a twelve-room world in Go. The
// difference between "a path that is hot" and "a path that looks hot" is
// a number, and this is where to get one.
//
// The counterpart lesson is in #326, which was filed off the same
// afternoon's reading and closed after these benchmarks said every item in
// it was three orders of magnitude below mattering. Measure first.

// benchWorld boots examples/stock — 2,981 rooms, 944 mobiles, 1,199
// objects, 47 zones, which is the size of the archived world
// (docs/investigations/lib-directory-format.md) — and resets it, so the
// benchmarks run against a populated world rather than an empty one.
func benchWorld(tb testing.TB) (*game.Live, *Server) {
	tb.Helper()

	src, err := world.Open(DataFormat, world.Config{Dir: "../../examples/stock/yaml/world"})
	if err != nil {
		tb.Fatal(err)
	}
	defs, err := src.Load(context.Background())
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = src.Close() }()

	live := game.NewLive(defs)
	source, err := rng.New("circle", 12345)
	if err != nil {
		tb.Fatal(err)
	}
	srv := &Server{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		rng:    rng.NewRand(source),
	}
	srv.BootReset(live)
	return live, srv
}

// BenchmarkBootReset is the whole world populated from nothing, which is
// what every boot does before anyone can connect.
func BenchmarkBootReset(b *testing.B) {
	src, err := world.Open(DataFormat, world.Config{Dir: "../../examples/stock/yaml/world"})
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	defs, err := src.Load(context.Background())
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		source, err := rng.New("circle", 12345)
		if err != nil {
			b.Fatal(err)
		}
		srv := &Server{
			logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			rng:    rng.NewRand(source),
		}
		srv.BootReset(game.NewLive(defs))
	}
}

// BenchmarkResetOneZone is the one that mattered: `drainZoneQueue` resets
// every due zone in a single task on the world goroutine, every ten
// seconds, against a 100ms budget. Zone 54, "New Thalos", is the stock
// world's largest at 401 reset commands.
func BenchmarkResetOneZone(b *testing.B) {
	live, srv := benchWorld(b)

	var biggest *game.ZoneDef
	for _, z := range live.Zones() {
		if biggest == nil || len(z.Commands) > len(biggest.Commands) {
			biggest = z
		}
	}
	b.Logf("zone %d %q, %d reset commands", biggest.Vnum, biggest.Name, len(biggest.Commands))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		live.ResetZone(biggest, srv.rng)
	}
}

// BenchmarkResetEveryZone is the worst case that actually happens: every
// zone starts at age 0 at boot and many share a lifespan, so they come due
// together and `drainZoneQueue` resets the lot in one pulse.
func BenchmarkResetEveryZone(b *testing.B) {
	live, srv := benchWorld(b)
	zones := live.Zones()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, z := range zones {
			live.ResetZone(z, srv.rng)
		}
	}
}

// BenchmarkMobileActivity is one pass over every mobile in the world, on
// the pulse that runs every ten seconds.
func BenchmarkMobileActivity(b *testing.B) {
	live, srv := benchWorld(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srv.mobileActivity(live)
	}
}

// BenchmarkPointUpdate is the mud-hourly tick: conditions, affect ageing,
// regeneration and the corpse decay pass over every object in the world.
// With no players it is the object half, which is the half that scales
// with how long the server has been up.
func BenchmarkPointUpdate(b *testing.B) {
	live, srv := benchWorld(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srv.pointUpdate(live)
	}
}

// BenchmarkZoneUpdate is the ageing pass with nothing due, which is what
// it does on all but one call in six.
func BenchmarkZoneUpdate(b *testing.B) {
	live, srv := benchWorld(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srv.zoneUpdate(live)
	}
}
