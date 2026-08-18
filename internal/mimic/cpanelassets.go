package mimic

import "embed"

// cpanelAssets holds the real cPanel/cpsrvd whitelabel login assets (the white cPanel
// wordmark, the username/password field icons, and the favicon), captured from a live
// cPanel login and served at the paths a genuine panel uses. cPanel, WHM and Webmail
// all share the same cpsrvd login template, so these back all three. Embedded in the
// binary — nothing to ship separately.
//
//go:embed cpanelassets/cpanel-logo.svg cpanelassets/icon-username.png cpanelassets/icon-password.png cpanelassets/favicon.ico
var cpanelAssets embed.FS
