package mimic

import "embed"

// grafanaAssets holds the real Grafana brand assets (the gradient icon logo and the
// 32px favicon), captured from a live Grafana frontend and served at Grafana's own
// /public/... paths so the decoy's asset requests match a genuine instance. Embedded
// in the binary — nothing to ship separately.
//
//go:embed grafanaassets/grafana_icon.svg grafanaassets/fav32.png
var grafanaAssets embed.FS
