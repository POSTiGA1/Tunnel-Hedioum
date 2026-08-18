package mimic

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

// cPanel / WHM / Webmail persona decoys for the :2083 / :2087 / :2096 TLS mimics. All
// three are served by cpsrvd from the same whitelabel login template, so one harness
// backs all three — a probe or browser that reaches the port without the tunnel token
// gets a faithful reproduction of the real cpsrvd login (blue gradient canvas, the
// white cPanel wordmark, icon-prefixed Username/Password fields, the blue "Log in"
// button), reproduced from a live cPanel login's computed styles and real embedded
// assets, and answered with cpsrvd's characteristic `Server: cpsrvd/...` header that
// scanners fingerprint cPanel by. Only the <title> differs between the three.
//
// Display-only camouflage on the shared panel harness: no credential is inspected,
// stored, or forwarded; a login attempt shows cpsrvd's real "The login is invalid."
// text and the page's script does not transmit the typed values.

// cpsrvdServer is the Server header a recent cPanel emits — the primary cPanel tell.
const cpsrvdServer = "cpsrvd/11.126.0.6"

// cpsrvdLoginPage is the rendered cpsrvd login. __PRODUCT__ becomes cPanel/WHM/Webmail.
const cpsrvdLoginPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>__PRODUCT__ Login</title>
<link rel="shortcut icon" href="/favicon.ico" />
<style>
  * { box-sizing: border-box; }
  html, body { margin: 0; padding: 0; height: 100%; }
  body {
    min-height: 100vh; font-family: "Open Sans", helvetica, arial, sans-serif; color: #333;
    background: linear-gradient(90deg, #011a62, #01376b);
    display: flex; flex-direction: column; align-items: center; justify-content: center;
  }
  .logo { width: 179px; height: 40px; margin-bottom: 26px; }
  .logo img { width: 179px; height: 40px; display: block; }
  .card {
    background: #fff; border-radius: 4px; width: 345px; max-width: 92vw; padding: 30px;
    box-shadow: 0 4px 16px 0 rgba(19,26,44,.02), 0 0 32px 0 rgba(19,26,44,.1);
  }
  form { margin: 0; }
  .field {
    display: flex; align-items: center; background: #fff; border: 1px solid #dcdee2;
    border-radius: 2px; margin-bottom: 14px; height: 44px; overflow: hidden;
  }
  .field .icon { width: 44px; height: 44px; flex: 0 0 44px; background-repeat: no-repeat;
    background-position: center; background-size: 20px 20px; }
  .field.user .icon { background-image: url("/unprotected/whitelabel/images/icon-username.png"); }
  .field.pass .icon { background-image: url("/unprotected/whitelabel/images/icon-password.png"); }
  .field input {
    flex: 1 1 auto; height: 42px; border: 0; outline: none; padding: 0 12px 0 4px;
    font-size: 14px; font-family: inherit; color: #333; background: transparent;
  }
  .field input::placeholder { color: #8a8a8a; }
  .field:focus-within { border-color: #1976d2; }
  .loginError { color: #c0392b; font-size: 13px; min-height: 16px; margin: 0 0 10px; }
  .loginbtn {
    width: 100%; height: 42px; border: 0; border-radius: 4px; background: #1976d2; color: #fff;
    font-size: 15px; font-family: inherit; cursor: pointer;
  }
  .loginbtn:hover { background: #1565c0; }
  .reset { text-align: center; margin-top: 16px; }
  .reset a { color: #fff; font-size: 13px; text-decoration: none; }
  .reset a:hover { text-decoration: underline; }
</style>
</head>
<body>
  <div class="logo"><img src="/unprotected/whitelabel/images/cpanel-logo.svg" alt="logo" /></div>
  <div class="card">
    <form id="loginForm" action="javascript:void(0)">
      <div class="field user"><span class="icon"></span><input id="user" name="user" type="text" placeholder="Enter your username." autocapitalize="none" autocomplete="username" /></div>
      <div class="field pass"><span class="icon"></span><input id="pass" name="pass" type="password" placeholder="Enter your account password." autocomplete="current-password" /></div>
      <div class="loginError" id="loginError"></div>
      <button class="loginbtn" type="submit">Log in</button>
    </form>
  </div>
  <div class="reset"><a href="/resetpass">Reset Password</a></div>
  <script>
    (function () {
      try {
        var f = document.getElementById("loginForm"), err = document.getElementById("loginError");
        if (f) f.addEventListener("submit", function (e) {
          e.preventDefault();
          // Credentials are never transmitted or stored; only the failed-login state is mimicked.
          try { fetch("/login/?login_only=1", { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" }, body: "" }); } catch (_) {}
          err.textContent = "The login is invalid.";
        });
      } catch (_) {}
    })();
  </script>
</body>
</html>
`

var cpanelAssetRoutes = map[string]struct{ file, ct string }{
	"/unprotected/whitelabel/images/cpanel-logo.svg":   {"cpanel-logo.svg", "image/svg+xml"},
	"/unprotected/whitelabel/images/icon-username.png": {"icon-username.png", "image/png"},
	"/unprotected/whitelabel/images/icon-password.png": {"icon-password.png", "image/png"},
	"/favicon.ico": {"favicon.ico", "image/x-icon"},
}

// ServeCPanel / ServeWHM / ServeWebmail are the three cpsrvd persona entry points.
func ServeCPanel(conn net.Conn)  { servePanel(conn, cpsrvdRoute("cPanel")) }
func ServeWHM(conn net.Conn)     { servePanel(conn, cpsrvdRoute("WHM")) }
func ServeWebmail(conn net.Conn) { servePanel(conn, cpsrvdRoute("Webmail")) }

func cpsrvdRoute(product string) panelRoute {
	return func(req *http.Request) panelResp {
		path := req.URL.Path
		if a, ok := cpanelAssetRoutes[path]; ok {
			return cpanelAsset(a.file, a.ct)
		}
		switch {
		case strings.HasPrefix(path, "/login") && req.Method == http.MethodPost:
			// A login attempt — answered exactly as cpsrvd answers a bad login.
			return cpanelJSON(200, "OK", `{"status":0,"errors":["The login is invalid."]}`)
		default:
			// The login page for /, /login and every other path.
			return cpanelHTML(strings.ReplaceAll(cpsrvdLoginPage, "__PRODUCT__", product))
		}
	}
}

// cpanelHeaders carries cpsrvd's characteristic headers: the cpsrvd Server tell, plus
// the no-store cache policy a login page uses.
func cpanelHeaders(contentType string, n int, cache bool) [][2]string {
	h := [][2]string{
		{"Server", cpsrvdServer},
		{"Content-Type", contentType},
		{"Content-Length", strconv.Itoa(n)},
	}
	if cache {
		h = append(h, [2]string{"Cache-Control", "max-age=31536000"})
	} else {
		h = append(h, [2]string{"Cache-Control", "no-cache, no-store, must-revalidate"})
	}
	return h
}

func cpanelHTML(body string) panelResp {
	return panelResp{status: 200, reason: "OK", middle: cpanelHeaders("text/html; charset=utf-8", len(body), false), body: []byte(body)}
}

func cpanelJSON(status int, reason, body string) panelResp {
	return panelResp{status: status, reason: reason, middle: cpanelHeaders("application/json", len(body), false), body: []byte(body)}
}

func cpanelAsset(file, ct string) panelResp {
	b, err := cpanelAssets.ReadFile("cpanelassets/" + file)
	if err != nil {
		return panelText(404, "Not Found", "text/html; charset=utf-8", "Not Found")
	}
	return panelResp{status: 200, reason: "OK", middle: cpanelHeaders(ct, len(b), true), body: b}
}
