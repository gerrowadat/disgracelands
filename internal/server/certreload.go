// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

// CertReloader serves a TLS certificate loaded from a cert/key file pair on
// disk, and keeps it current: [CertReloader.Run] polls the files for a
// newer mtime and reloads when it finds one, so a certificate renewed in
// place — a cert-manager/certbot rotation, or an ops team's own cron — takes
// effect on the next handshake instead of needing a restart to be noticed
// (issue #147: before this existed, the keypair was read once at boot with
// tls.LoadX509KeyPair and never looked at again).
//
// The C server predates TLS entirely, so there is no C behaviour to match
// or diverge from here — this is pure operational plumbing, not a
// deviation.
//
// crypto/tls asks for the certificate to present through
// tls.Config.GetCertificate on every handshake, which is what makes the
// swap live without dropping anything: [CertReloader.GetCertificate] only
// ever reads a pointer, [CertReloader.reload] only ever replaces it
// atomically, and a handshake already in flight sees whichever certificate
// was current when it asked — never a half-updated one, and nothing already
// connected is touched at all.
type CertReloader struct {
	certFile, keyFile string
	logger            *slog.Logger

	current atomic.Pointer[tls.Certificate]
	// modTime is the newer of the two files' mtimes as of the certificate
	// currently loaded. Touched only from reload, which Run calls from a
	// single goroutine, so unlike current it needs no synchronization of
	// its own.
	modTime time.Time
}

// NewCertReloader loads the keypair once, synchronously: a bad path or an
// unparsable keypair is a boot failure here exactly as it was before this
// existed. Only a *later* failure, from [CertReloader.Run], is swallowed
// and logged — the same posture cmd/dlmud's SIGHUP handling takes with a
// bad --config file, because a mistake in a file on disk must never be able
// to take down a game (or a certificate) that is already up.
func NewCertReloader(certFile, keyFile string, logger *slog.Logger) (*CertReloader, error) {
	r := &CertReloader{certFile: certFile, keyFile: keyFile, logger: logger}
	cert, modTime, err := loadKeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	r.current.Store(cert)
	r.modTime = modTime
	return r, nil
}

// GetCertificate is a tls.Config.GetCertificate callback: the certificate
// currently loaded, for every handshake.
func (r *CertReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return r.current.Load(), nil
}

// Run polls the cert and key files for a change every interval, reloading
// when it finds one, until ctx is cancelled. An interval of zero or less
// disables polling entirely — the certificate loaded at construction is
// used for the life of the process, matching this server's behaviour
// before this existed.
func (r *CertReloader) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reload()
		}
	}
}

// reload checks whether either file is newer than the certificate currently
// loaded and, if so, reloads it. Any error — the files disappearing, one of
// the pair not matching, a corrupt PEM block — is logged and the
// certificate already serving connections is left in place; it is never
// allowed to bring the listener down.
func (r *CertReloader) reload() {
	modTime, err := newestModTime(r.certFile, r.keyFile)
	if err != nil {
		r.logger.Error("checking the TLS certificate for changes",
			"cert", r.certFile, "key", r.keyFile, "error", err)
		return
	}
	if !modTime.After(r.modTime) {
		return
	}
	cert, newModTime, err := loadKeyPair(r.certFile, r.keyFile)
	if err != nil {
		r.logger.Error("reloading the TLS certificate; keeping the one already in use",
			"cert", r.certFile, "key", r.keyFile, "error", err)
		return
	}
	r.current.Store(cert)
	r.modTime = newModTime
	r.logger.Info("reloaded the TLS certificate", "cert", r.certFile, "key", r.keyFile)
}

// loadKeyPair reads a certificate/key pair and the newer of their two
// mtimes together, so a caller never stores one without the other.
func loadKeyPair(certFile, keyFile string) (*tls.Certificate, time.Time, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("loading the TLS certificate: %w", err)
	}
	modTime, err := newestModTime(certFile, keyFile)
	if err != nil {
		return nil, time.Time{}, err
	}
	return &cert, modTime, nil
}

// newestModTime is the later of the given files' modification times, so a
// change to either the certificate or the key is noticed.
func newestModTime(paths ...string) (time.Time, error) {
	var newest time.Time
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return time.Time{}, fmt.Errorf("stat %s: %w", p, err)
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest, nil
}
