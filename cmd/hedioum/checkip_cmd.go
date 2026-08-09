package main

import (
	"net/http"
	"time"

	"github.com/fatih/color"
	"github.com/hedioum/Hedioum-Pool-Tunnel/internal/ipcheck"
)

// cmdCheckIP reports the server's egress public IP and a reputation verdict, so an
// operator can judge an IP before (or after) deploying a foreign node.
func cmdCheckIP(args []string) {
	color.HiBlue("\n--- Egress IP Reputation Check ---")
	color.HiBlack("    (best-effort; a LIKELY-FLAGGED verdict means AI sites / some CDNs may block this IP)")

	client := &http.Client{Timeout: 12 * time.Second}
	rep := ipcheck.Run(client, ipcheck.DefaultConfig())

	if rep.PublicIP != "" {
		color.HiWhite("\n  Public IP : %s", rep.PublicIP)
	} else {
		color.Yellow("\n  Public IP : (could not determine)")
	}
	if rep.Org != "" {
		color.HiWhite("  Network   : %s", rep.Org)
	}

	for _, s := range rep.Signals {
		switch {
		case s.Err != "":
			color.HiBlack("   ·  %-18s unreachable (%s)", s.Name, s.Err)
		case s.Status == 403:
			color.Red("   ✗  %-18s HTTP 403 (blocked)", s.Name)
		default:
			color.Green("   ✓  %-18s HTTP %d", s.Name, s.Status)
		}
	}

	switch rep.Verdict {
	case ipcheck.Clean:
		color.HiGreen("\n  Verdict   : CLEAN — no reputation blocks detected.")
	case ipcheck.LikelyFlagged:
		color.HiRed("\n  Verdict   : LIKELY-FLAGGED — consider a cleaner IP or CDN fronting.")
	default:
		color.HiYellow("\n  Verdict   : UNKNOWN — could not assess (network/connectivity).")
	}
	for _, e := range rep.Evidence {
		color.HiBlack("    - %s", e)
	}
}
