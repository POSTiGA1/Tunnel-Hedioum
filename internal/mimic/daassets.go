package mimic

import "embed"

// daAssets holds the real DirectAdmin "Evolution" login assets (logo, branded
// background, favicon), captured verbatim from a live panel and served at their
// original /evo/assets/ paths so the decoy's asset requests match a genuine panel.
//
//go:embed daassets/logo.fe968txS.svg daassets/background.Cx34YJbp.svg daassets/favicon.CDLA4ANV.png
var daAssets embed.FS
