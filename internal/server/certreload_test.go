// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSelfSignedCertFiles writes a PEM certificate/key pair to certFile and
// keyFile, with commonName baked in so a test can tell two generations of
// certificate apart after loading one back with tls.LoadX509KeyPair (whose
// returned tls.Certificate carries the parsed Leaf).
func writeSelfSignedCertFiles(t *testing.T, certFile, keyFile, commonName string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(certFile,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile,
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
}

// commonNameOf reads back the CommonName a CertReloader is currently
// serving, so a test can tell which generation of certificate is live
// without reaching into its unexported fields.
func commonNameOf(t *testing.T, r *CertReloader) string {
	t.Helper()
	cert, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf.Subject.CommonName
}

func testCertLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestCertReloaderPicksUpARenewedCertificate is the fix for issue #147: a
// certificate renewed on disk in place — the shape a cert-manager/certbot
// rotation or an ops cron takes, overwriting the same two paths — must be
// picked up without a restart.
func TestCertReloaderPicksUpARenewedCertificate(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	writeSelfSignedCertFiles(t, certFile, keyFile, "v1")
	r, err := NewCertReloader(certFile, keyFile, testCertLogger())
	if err != nil {
		t.Fatal(err)
	}
	if got := commonNameOf(t, r); got != "v1" {
		t.Fatalf("commonNameOf() = %q, want %q", got, "v1")
	}

	// mtime resolution on some filesystems is coarse (1s on many); back-date
	// the original pair and forward-date the renewal explicitly rather than
	// relying on wall-clock time to have moved between the two writes.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(certFile, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(keyFile, past, past); err != nil {
		t.Fatal(err)
	}
	r.modTime = past.Add(-time.Second)

	writeSelfSignedCertFiles(t, certFile, keyFile, "v2")

	r.reload()

	if got := commonNameOf(t, r); got != "v2" {
		t.Fatalf("commonNameOf() after reload = %q, want %q (renewal was not picked up)", got, "v2")
	}
}

// TestCertReloaderDoesNothingWhenNothingChanged: reload is on a poll
// timer, called whether or not the files actually moved, so it must be a
// no-op — not just "loads the same bytes again" but literally does not
// touch the certificate in use — when neither file's mtime has advanced.
func TestCertReloaderDoesNothingWhenNothingChanged(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	writeSelfSignedCertFiles(t, certFile, keyFile, "only")
	r, err := NewCertReloader(certFile, keyFile, testCertLogger())
	if err != nil {
		t.Fatal(err)
	}
	before := r.current.Load()

	r.reload()

	if after := r.current.Load(); after != before {
		t.Error("reload() replaced the certificate even though neither file's mtime changed")
	}
}

// TestCertReloaderKeepsTheOldCertificateOnAFailedReload: a bad write to the
// certificate file mid-rotation — truncated, wrong permissions, a key that
// no longer matches the cert — must never be able to take down a listener
// that is already serving connections. The old certificate stays live and
// the error is logged, not raised.
func TestCertReloaderKeepsTheOldCertificateOnAFailedReload(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")

	writeSelfSignedCertFiles(t, certFile, keyFile, "good")
	r, err := NewCertReloader(certFile, keyFile, testCertLogger())
	if err != nil {
		t.Fatal(err)
	}

	future := time.Now().Add(time.Hour)
	if err := os.WriteFile(certFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(certFile, future, future); err != nil {
		t.Fatal(err)
	}

	r.reload()

	if got := commonNameOf(t, r); got != "good" {
		t.Fatalf("commonNameOf() after a failed reload = %q, want the original %q kept", got, "good")
	}
}

// TestCertReloaderRunStopsOnContextCancellation: Run must not leak past its
// context, the same discipline every other background loop main starts
// follows.
func TestCertReloaderRunStopsOnContextCancellation(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	writeSelfSignedCertFiles(t, certFile, keyFile, "v1")

	r, err := NewCertReloader(certFile, keyFile, testCertLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Run(ctx, time.Millisecond)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

// TestCertReloaderRunPicksUpAChangeOnItsOwn exercises Run end-to-end rather
// than calling reload directly, on a real (short) timer.
func TestCertReloaderRunPicksUpAChangeOnItsOwn(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	writeSelfSignedCertFiles(t, certFile, keyFile, "v1")

	r, err := NewCertReloader(certFile, keyFile, testCertLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go r.Run(ctx, 10*time.Millisecond)

	future := time.Now().Add(time.Hour)
	writeSelfSignedCertFiles(t, certFile, keyFile, "v2")
	if err := os.Chtimes(certFile, future, future); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(keyFile, future, future); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if commonNameOf(t, r) == "v2" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Run did not pick up the renewed certificate within the deadline")
}

// TestNewCertReloaderFailsOnABadKeypair: the same posture tlsConfig had
// before this existed — a boot-time misconfiguration is a boot failure, not
// something to start serving around.
func TestNewCertReloaderFailsOnABadKeypair(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := NewCertReloader(certFile, keyFile, testCertLogger()); err == nil {
		t.Fatal("NewCertReloader with an unparsable keypair returned no error")
	}
}
