package mimic

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

// Grafana persona decoy for the :3000 TLS mimic. A probe or browser that reaches the
// port without the tunnel token is served a faithful reproduction of the default
// self-hosted Grafana login — the real gradient icon, dark theme, "Welcome to
// Grafana" heading, username/password form and "Log in" button, reproduced from a
// live Grafana 11 frontend's computed styles — plus the genuine unauthenticated
// /api/health JSON that scanners fingerprint Grafana by. So the port reads as an
// ordinary Grafana instance to both scanners and humans.
//
// It is display-only camouflage: it never validates, stores, or forwards any
// submitted credentials; a login attempt shows Grafana's real error text and the
// page's script does not transmit the typed values. Built on the shared panel
// harness (panel.go); Grafana's frontend is plain HTTP/1.1.

// grafanaVersion is advertised by /api/health — a real recent OSS release string.
const grafanaVersion = "11.3.0"

// grafanaLoginPage is the rendered default Grafana login (dark theme), reproduced to
// match a live instance: canvas #111217, translucent card #181B1F @70% radius 10px,
// #CCCCDC text, Inter, and the #3D71D9 primary button.
const grafanaLoginPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>Grafana</title>
<link rel="icon" type="image/png" href="/public/img/fav32.png" />
<style>
  :root {
    --bg: #111217; --card: rgba(24,27,31,.7); --text: #ccccdc; --text-weak: rgba(204,204,220,.65);
    --border: rgba(204,204,220,.2); --input-bg: #111217; --blue: #3d71d9; --blue-hover: #6e9fff;
  }
  * { box-sizing: border-box; }
  html, body { margin: 0; padding: 0; height: 100%; }
  body {
    min-height: 100vh; background: var(--bg); color: var(--text);
    font-family: Inter, Helvetica, Arial, sans-serif; font-size: 14px;
    display: flex; align-items: center; justify-content: center;
  }
  .LoginCard {
    background: var(--card); border-radius: 10px; width: 478px; max-width: 94vw;
    padding: 40px 24px 24px; display: flex; flex-direction: column; align-items: center;
  }
  .LoginLogo { width: 64px; height: 64px; margin-bottom: 24px; }
  .LoginTitle { font-size: 32px; font-weight: 400; color: var(--text); margin: 0 0 24px; text-align: center; }
  form { width: 383px; max-width: 100%; display: flex; flex-direction: column; }
  label { font-size: 12px; font-weight: 500; color: var(--text); margin: 0 0 4px; }
  form label:not(:first-child) { margin-top: 16px; }
  .Field { position: relative; }
  input {
    width: 100%; height: 32px; background: var(--input-bg); color: var(--text);
    border: 1px solid var(--border); border-radius: 6px; padding: 0 8px; font-size: 14px;
    font-family: inherit; outline: none;
  }
  input::placeholder { color: var(--text-weak); }
  input:focus { border-color: var(--blue); }
  .Eye { position: absolute; top: 0; right: 0; height: 32px; width: 32px; display: flex;
    align-items: center; justify-content: center; color: var(--text-weak); cursor: pointer; background: none; border: 0; }
  .Eye svg { width: 16px; height: 16px; }
  .LoginError { color: #ff5286; font-size: 13px; min-height: 16px; margin: 12px 0 0; }
  .LoginButton {
    margin: 24px 0 0; height: 32px; width: 100%; border: 0; border-radius: 6px;
    background: var(--blue); color: #fff; font-size: 14px; font-weight: 500; font-family: inherit; cursor: pointer;
  }
  .LoginButton:hover { background: var(--blue-hover); }
  .ForgotRow { display: flex; justify-content: center; margin-top: 16px; }
  .ForgotRow a { color: var(--text); font-size: 14px; text-decoration: none; }
  .ForgotRow a:hover { text-decoration: underline; }
</style>
</head>
<body>
  <div class="LoginCard">
    <img class="LoginLogo" src="/public/build/static/img/grafana_icon.svg" alt="Grafana" />
    <h1 class="LoginTitle">Welcome to Grafana</h1>
    <form id="loginForm" autocomplete="off">
      <label for="user">Email or username</label>
      <div class="Field">
        <input id="user" name="user" type="text" placeholder="email or username" autocapitalize="none" />
      </div>
      <label for="password">Password</label>
      <div class="Field">
        <input id="password" name="password" type="password" placeholder="password" autocomplete="current-password" />
        <button class="Eye" type="button" aria-label="Show password">
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M21.92,11.6C19.9,6.91,16.1,4,12,4S4.1,6.91,2.08,11.6a1,1,0,0,0,0,.8C4.1,17.09,7.9,20,12,20s7.9-2.91,9.92-7.6A1,1,0,0,0,21.92,11.6ZM12,18c-3.17,0-6.17-2.29-7.9-6C5.83,8.29,8.83,6,12,6s6.17,2.29,7.9,6C18.17,15.71,15.17,18,12,18ZM12,8a4,4,0,1,0,4,4A4,4,0,0,0,12,8Zm0,6a2,2,0,1,1,2-2A2,2,0,0,1,12,14Z"/></svg>
        </button>
      </div>
      <div class="LoginError" id="loginError"></div>
      <button class="LoginButton" type="submit">Log in</button>
    </form>
    <div class="ForgotRow"><a href="/user/password/send-reset-email">Forgot your password?</a></div>
  </div>
  <script>
    (function () {
      try {
        var eye = document.querySelector(".Eye"), pw = document.getElementById("password");
        if (eye && pw) eye.addEventListener("click", function () { pw.type = pw.type === "password" ? "text" : "password"; });
        var f = document.getElementById("loginForm"), err = document.getElementById("loginError");
        if (f) f.addEventListener("submit", function (e) {
          e.preventDefault();
          // Credentials are never transmitted or stored; only the failed-login state is mimicked.
          try { fetch("/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" }); } catch (_) {}
          err.textContent = "Invalid username or password";
        });
      } catch (_) {}
    })();
  </script>
</body>
</html>
`

// ServeGrafana handles one (keep-alive) connection as a Grafana instance would.
func ServeGrafana(conn net.Conn) { servePanel(conn, routeGrafana) }

var grafanaAssetRoutes = map[string]struct{ file, ct string }{
	"/public/build/static/img/grafana_icon.svg": {"grafana_icon.svg", "image/svg+xml"},
	"/public/img/fav32.png":                     {"fav32.png", "image/png"},
	"/favicon.ico":                              {"fav32.png", "image/png"},
}

func routeGrafana(req *http.Request) panelResp {
	path := req.URL.Path
	if a, ok := grafanaAssetRoutes[path]; ok {
		return panelAssetResp(grafanaAssets, "grafanaassets", a.file, a.ct)
	}
	switch {
	case path == "/":
		return grafanaRedirect("/login")
	case path == "/api/health":
		// The unauthenticated health endpoint scanners fingerprint Grafana by.
		return grafanaJSON(200, "OK", `{"commit":"na","database":"ok","version":"`+grafanaVersion+`"}`)
	case strings.HasPrefix(path, "/api/"):
		return grafanaJSON(401, "Unauthorized", `{"message":"Unauthorized"}`)
	default:
		// The login SPA for /login and every other route.
		return grafanaHTML(200, "OK", grafanaLoginPage)
	}
}

// grafanaBody carries Grafana's characteristic headers: no Server header, plus
// X-Content-Type-Options / X-Frame-Options: deny / Cache-Control: no-store.
func grafanaBody(status int, reason, contentType, body string) panelResp {
	return panelResp{
		status: status, reason: reason,
		middle: [][2]string{
			{"Content-Type", contentType},
			{"Content-Length", strconv.Itoa(len(body))},
			{"Cache-Control", "no-store"},
			{"X-Content-Type-Options", "nosniff"},
			{"X-Frame-Options", "deny"},
		},
		body: []byte(body),
	}
}

func grafanaJSON(status int, reason, body string) panelResp {
	return grafanaBody(status, reason, "application/json", body)
}

func grafanaHTML(status int, reason, body string) panelResp {
	return grafanaBody(status, reason, "text/html; charset=UTF-8", body)
}

func grafanaRedirect(location string) panelResp {
	return panelResp{
		status: 302, reason: "Found",
		middle: [][2]string{
			{"Location", location},
			{"Content-Type", "text/html; charset=utf-8"},
			{"Content-Length", "0"},
			{"X-Content-Type-Options", "nosniff"},
		},
	}
}
