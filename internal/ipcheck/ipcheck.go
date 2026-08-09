// Package ipcheck assesses the reputation of the server's egress public IP, so an
// operator can tell before/after deploying whether the address is a "clean" range or
// a pre-flagged datacenter IP (which silently degrades AI sites, some CDNs, etc.).
// The verdict logic is pure and unit-tested; gathering is best-effort and never
// fatal.
package ipcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Verdict values.
const (
	Clean         = "CLEAN"
	LikelyFlagged = "LIKELY-FLAGGED"
	Unknown       = "UNKNOWN"
)

// Signal is one probe result. Category is "baseline" (plain connectivity) or
// "reputation" (a service known to 403 flagged datacenter IPs).
type Signal struct {
	Name     string
	Category string
	Status   int
	Err      string
}

// Report is the full outcome of a check.
type Report struct {
	PublicIP string
	Org      string
	Signals  []Signal
	Verdict  string
	Evidence []string
}

// NamedURL pairs a display name with a URL to probe.
type NamedURL struct{ Name, URL string }

// Config lists the endpoints to use; DefaultConfig is production, tests inject their own.
type Config struct {
	IPURL       string
	OrgURL      string
	BaselineURL string
	Reputation  []NamedURL
}

// DefaultConfig returns the production endpoints.
func DefaultConfig() Config {
	return Config{
		IPURL:       "https://api.ipify.org",
		OrgURL:      "https://ip-api.com/json/?fields=status,org,isp,as",
		BaselineURL: "https://www.google.com/",
		Reputation: []NamedURL{
			{"Google AI Studio", "https://aistudio.google.com/"},
			{"Gemini", "https://gemini.google.com/app"},
		},
	}
}

// Verdict derives the reputation verdict + human-readable evidence from raw signals.
// It is deliberately pure so it can be unit-tested without any network.
func Verdict(sigs []Signal) (string, []string) {
	var evidence []string
	baselineOK := false
	reputationChecked := 0
	flagged := 0

	for _, s := range sigs {
		switch s.Category {
		case "baseline":
			if s.Err == "" && s.Status >= 200 && s.Status < 400 {
				baselineOK = true
			}
		case "reputation":
			if s.Err != "" {
				continue // couldn't reach it — inconclusive, ignore
			}
			reputationChecked++
			if s.Status == 403 {
				flagged++
				evidence = append(evidence, fmt.Sprintf("%s returned 403 (blocked — datacenter/flagged IP)", s.Name))
			} else {
				evidence = append(evidence, fmt.Sprintf("%s returned %d", s.Name, s.Status))
			}
		}
	}

	if !baselineOK {
		return Unknown, append([]string{"baseline connectivity failed; cannot assess the IP"}, evidence...)
	}
	if flagged > 0 {
		return LikelyFlagged, evidence
	}
	if reputationChecked > 0 {
		return Clean, evidence
	}
	return Unknown, append(evidence, "no reputation endpoint was reachable")
}

// Run gathers all signals with the given client and config, then computes the verdict.
func Run(client *http.Client, cfg Config) Report {
	rep := Report{
		PublicIP: fetchText(client, cfg.IPURL),
		Org:      fetchOrg(client, cfg.OrgURL),
	}
	rep.Signals = append(rep.Signals, statusSignal(client, "baseline", "connectivity", cfg.BaselineURL))
	for _, r := range cfg.Reputation {
		rep.Signals = append(rep.Signals, statusSignal(client, "reputation", r.Name, r.URL))
	}
	rep.Verdict, rep.Evidence = Verdict(rep.Signals)
	return rep
}

func get(client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	return client.Do(req)
}

func statusSignal(client *http.Client, category, name, url string) Signal {
	resp, err := get(client, url)
	if err != nil {
		return Signal{Name: name, Category: category, Err: err.Error()}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	return Signal{Name: name, Category: category, Status: resp.StatusCode}
}

func fetchText(client *http.Client, url string) string {
	resp, err := get(client, url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return strings.TrimSpace(string(b))
}

func fetchOrg(client *http.Client, url string) string {
	resp, err := get(client, url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var v struct {
		Status, Org, ISP, AS string
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&v) != nil {
		return ""
	}
	parts := make([]string, 0, 3)
	for _, s := range []string{v.AS, v.Org, v.ISP} {
		if s != "" && !containsFold(parts, s) {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " · ")
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
