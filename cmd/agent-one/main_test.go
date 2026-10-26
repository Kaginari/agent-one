package main

import (
	"crypto/x509"
	"os"
	"os/exec"
	"testing"
)

// A static binary in a container without ca-certificates still verifies TLS: the system roots
// are empty there, so the embedded fallback bundle stands in. The pool is read in a child
// process because the system roots load once, from SSL_CERT_FILE/SSL_CERT_DIR at first use.
func TestFallbackRootsWhenTheHostHasNone(t *testing.T) {
	if os.Getenv("AGENT_ONE_ROOTS_CHILD") == "1" {
		pool, err := x509.SystemCertPool()
		if err != nil {
			t.Fatalf("system pool: %v", err)
		}
		if n := len(pool.Subjects()); n == 0 { //nolint:staticcheck // the only count a pool offers; the fallback pool is not a system pool
			t.Fatal("no roots on a host without CA certificates: TLS to any provider would fail")
		}
		return
	}
	empty := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestFallbackRootsWhenTheHostHasNone$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AGENT_ONE_ROOTS_CHILD=1", "SSL_CERT_FILE="+empty+"/none.pem", "SSL_CERT_DIR="+empty)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
}
