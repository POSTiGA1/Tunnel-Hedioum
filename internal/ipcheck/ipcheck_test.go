package ipcheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerdictClean(t *testing.T) {
	v, ev := Verdict([]Signal{
		{Category: "baseline", Status: 200},
		{Category: "reputation", Name: "AI", Status: 200},
		{Category: "reputation", Name: "Gemini", Status: 200},
	})
	if v != Clean {
		t.Fatalf("verdict = %q, want CLEAN (%v)", v, ev)
	}
}

func TestVerdictFlagged(t *testing.T) {
	v, ev := Verdict([]Signal{
		{Category: "baseline", Status: 200},
		{Category: "reputation", Name: "AI", Status: 403},
		{Category: "reputation", Name: "Gemini", Status: 200},
	})
	if v != LikelyFlagged {
		t.Fatalf("verdict = %q, want LIKELY-FLAGGED", v)
	}
	found := false
	for _, e := range ev {
		if e == "AI returned 403 (blocked — datacenter/flagged IP)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("evidence missing the 403 line: %v", ev)
	}
}

func TestVerdictUnknownNoBaseline(t *testing.T) {
	// Baseline unreachable -> can't assess even if a reputation endpoint answered.
	v, _ := Verdict([]Signal{
		{Category: "baseline", Err: "dial timeout"},
		{Category: "reputation", Name: "AI", Status: 200},
	})
	if v != Unknown {
		t.Fatalf("verdict = %q, want UNKNOWN", v)
	}
}

func TestVerdictUnknownNoReputation(t *testing.T) {
	// Baseline OK but every reputation endpoint errored -> inconclusive.
	v, _ := Verdict([]Signal{
		{Category: "baseline", Status: 200},
		{Category: "reputation", Name: "AI", Err: "reset"},
	})
	if v != Unknown {
		t.Fatalf("verdict = %q, want UNKNOWN", v)
	}
}

// TestRunAgainstMockServers exercises Run end-to-end without the real internet: a
// flagged reputation endpoint (403) with a healthy baseline yields LIKELY-FLAGGED,
// and the public IP / org are captured.
func TestRunAgainstMockServers(t *testing.T) {
	ip := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("203.0.113.7\n"))
	}))
	defer ip.Close()
	org := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","org":"OVH SAS","isp":"OVH","as":"AS16276 OVH SAS"}`))
	}))
	defer org.Close()
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer base.Close()
	flagged := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer flagged.Close()

	rep := Run(&http.Client{Timeout: 5 * time.Second}, Config{
		IPURL:       ip.URL,
		OrgURL:      org.URL,
		BaselineURL: base.URL,
		Reputation:  []NamedURL{{"MockAI", flagged.URL}},
	})
	if rep.PublicIP != "203.0.113.7" {
		t.Fatalf("public IP = %q", rep.PublicIP)
	}
	if rep.Org == "" {
		t.Fatalf("org not captured")
	}
	if rep.Verdict != LikelyFlagged {
		t.Fatalf("verdict = %q, want LIKELY-FLAGGED", rep.Verdict)
	}
}
