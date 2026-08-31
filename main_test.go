package main

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

// occupy takes ports so the hunt has to step over them.
func occupy(t *testing.T, from, to int) {
	t.Helper()
	for port := from; port <= to; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Skipf("port %d is already in use on this machine", port)
		}
		t.Cleanup(func() { ln.Close() })
	}
}

func TestListenBindsLoopbackOnly(t *testing.T) {
	// If something else on this machine already holds 8765 - a copy of the
	// register left running - the second half of this test is not answerable.
	if probe, err := net.Listen("tcp", "127.0.0.1:8765"); err != nil {
		t.Skip("port 8765 is already in use on this machine")
	} else {
		probe.Close()
	}

	ln, err := listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Errorf("listening on %q, which is not loopback only", addr)
	}
	if addr != "127.0.0.1:8765" {
		t.Errorf("listening on %q, want 127.0.0.1:8765 when it is free", addr)
	}
}

func TestListenSkipsBusyPort(t *testing.T) {
	occupy(t, 8765, 8766)

	ln, err := listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	if got := ln.Addr().String(); got != "127.0.0.1:8767" {
		t.Errorf("listening on %q, want 127.0.0.1:8767", got)
	}
}

func TestListenGivesUpAfterRange(t *testing.T) {
	occupy(t, 8765, 8785)

	ln, err := listen()
	if err == nil {
		ln.Close()
		t.Fatal("listen found a port with the whole range busy")
	}
	if !strings.Contains(err.Error(), "8765") || !strings.Contains(err.Error(), "8785") {
		t.Errorf("the error %q does not name the range", err)
	}
}

func TestConsoleBannerContainsAddress(t *testing.T) {
	banner := consoleBanner("127.0.0.1:8767")

	if !strings.Contains(banner, "http://127.0.0.1:8767") {
		t.Errorf("the console banner does not carry the address:\n%s", banner)
	}
	if !strings.Contains(banner, "Leave this window open. If you close it, the register stops.") {
		t.Errorf("the console banner does not carry the warning:\n%s", banner)
	}

	// Acceptance criterion 7 greps for the address alone on its own line,
	// indented by two spaces.
	found := false
	for _, line := range strings.Split(banner, "\n") {
		if line == "  http://127.0.0.1:8767" {
			found = true
		}
	}
	if !found {
		t.Errorf("no line holds the address by itself:\n%s", banner)
	}
}
