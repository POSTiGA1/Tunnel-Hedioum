package mimic

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// DirectAdmin persona decoy for the :2222 panel. A probe or browser that reaches
// :2222 (but does not present the tunnel token) is served a faithful reproduction
// of the DirectAdmin "Evolution" login — the real logo, branded background, favicon,
// class names, layout, and the exact response headers a live panel returns — so the
// box reads as an ordinary DirectAdmin server to both scanners and humans.
//
// It is display-only camouflage: it never validates, stores, or forwards any
// submitted credentials. A login attempt is answered like a real bad login
// ("Invalid credentials"); the page's script does not transmit the typed values.
//
// Behaviour mirrored from a live panel:
//   - GET /                       -> 302 to /evo/
//   - GET /evo/ , /evo/login, ... -> the login page (200, no-cache)
//   - GET /evo/assets/...         -> the embedded logo/background/favicon
//   - /api/*                      -> DirectAdmin-shaped JSON (401 for auth)
//
// For pixel-perfect interactive fidelity an operator can instead point the mimic's
// Decoy backend at a real DirectAdmin instance.

// daLoginPage is the rendered Evolution login, captured from a live panel (real
// class names + computed styles) and made self-contained. __HOST__/__DATETIME__ are
// filled per request.
const daLoginPage = `<!DOCTYPE html>
<html class="vue-app" lang="en">
    <head>
        <meta http-equiv="Content-Type" content="text/html; charset=utf-8;" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <title>__HOST__ | Login</title>
        <link id="favicon" rel="shortcut icon" href="/evo/assets/favicon.CDLA4ANV.png" />
        <style>
            @font-face { font-family: "Montserrat"; src: local("Montserrat"); }
            * { box-sizing: border-box; }
            html, body { margin: 0; padding: 0; }
            body {
                min-height: 100vh;
                font-family: "Montserrat", system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
                background-color: #ebebef;
                background-image: url("/evo/assets/background.Cx34YJbp.svg");
                background-size: cover;
                background-position: center;
                background-repeat: no-repeat;
            }
            #EvoLoginApp { min-height: 100vh; display: flex; align-items: center; justify-content: center; }
            .Overlay { width: 100%; display: flex; align-items: center; justify-content: center; }
            .Wrapper { width: 500px; max-width: 92vw; }
            .Login.Box { background: transparent; }
            .Box__Header {
                height: 120px; width: 400px; max-width: 100%;
                margin: 0 0 14px 50px;
                background-image: url("/evo/assets/logo.fe968txS.svg");
                background-size: contain; background-repeat: no-repeat; background-position: 50% 50%;
            }
            .Box__Form { display: flex; flex-direction: column; }
            .InputLabel { color: #1d293b; font-size: 20px; font-weight: 500; letter-spacing: 0.24px; margin: 0; }
            .Box__Form label:not(:first-child) { margin-top: 16px; }
            .Input { position: relative; margin: 6px 0 0; }
            .Input__Text, .InputPassword__Input {
                width: 100%; height: 72.5px;
                border: 1.25px solid #e5e7eb; border-radius: 10px;
                padding: 15px 19px; font-size: 18px; color: #1f2937;
                background: #fff; font-family: inherit; outline: none;
            }
            .Input__Text:focus, .InputPassword__Input:focus { border-color: #2c8ec4; }
            .Input__Icon {
                position: absolute; top: 0; right: 25px; height: 72.5px;
                display: flex; align-items: center; color: #94a3b8; cursor: pointer;
            }
            .LoginError { color: #dc2626; font-size: 15px; min-height: 18px; margin: 10px 0 0; text-align: center; }
            .Button {
                margin: 28px 0 0; height: 80px; width: 100%; border: 0; border-radius: 10px;
                background: #2c8ec4; color: #fff; font-size: 20px; font-weight: 500;
                letter-spacing: 0.24px; font-family: inherit; cursor: pointer;
            }
            .DateTimeBlock { color: #fff; font-size: 13px; text-align: center; margin: 18px 0 0; text-shadow: 0 1px 2px rgba(0,0,0,.35); }
            .Toolbar { position: fixed; bottom: 18px; left: 0; right: 0; display: flex; align-items: center; justify-content: center; gap: 26px; color: #fff; }
            .ModeToggle { display: flex; align-items: center; gap: 14px; opacity: .9; }
            .ModeToggle .icon svg { width: 22px; height: 22px; }
            .ModeToggle .icon { opacity: .6; }
            .ModeToggle .icon.--active { opacity: 1; }
            .LanguageDropdown .LD__Button { display: flex; align-items: center; gap: 8px; font-size: 14px; cursor: pointer; }
        </style>
    </head>
    <body class="--logged-out">
        <div id="root" data-v-app>
            <div id="EvoLoginApp">
                <section class="Overlay">
                    <div class="Wrapper">
                        <div class="Login Box --logged-out">
                            <div class="Box__Header"></div>
                            <form id="LoginForm" class="Box__Form" autocomplete="on">
                                <label class="InputLabel" for="username">Username</label>
                                <div class="Input">
                                    <input id="username" name="username" class="Input__Text" type="text" placeholder="Please enter username" autocomplete="username" />
                                </div>
                                <label class="InputLabel" for="password">Password</label>
                                <div class="Input InputPassword">
                                    <input id="password" name="password" class="InputPassword__Input" type="password" placeholder="Please enter your password" autocomplete="current-password" />
                                    <a class="Input__Icon hoverable" role="button" aria-label="Show password"><svg viewBox="0 0 48 48" width="20" height="20"><path fill="currentColor" d="M24 8C11.056 8 0 24 0 24s11.048 16 24 16c12.944 0 24-16 24-16S36.952 8 24 8zm0 28c-7.469 0-15.137-7.324-19-12 3.853-4.678 11.502-12 19-12 7.469 0 15.137 7.324 19 12-3.853 4.678-11.502 12-19 12zm0-20a8 8 0 1 0 0 16 8 8 0 0 0 0-16zm0 12c-2.451 0-4-1.549-4-4s1.549-4 4-4 4 1.549 4 4-1.549 4-4 4z"/></svg></a>
                                </div>
                                <div class="LoginError" id="loginError"></div>
                                <button class="Button" type="submit"><span>Sign in</span></button>
                            </form>
                            <div class="DateTimeBlock">__DATETIME__</div>
                        </div>
                    </div>
                </section>
                <div class="Toolbar">
                    <div class="ModeToggle">
                        <span class="icon" aria-label="Light Mode"><svg viewBox="0 0 219.786 219.786"><path fill="#fff" d="M109.881 183.46a7.5 7.5 0 0 0-7.5 7.5v21.324a7.5 7.5 0 0 0 15 0V190.96a7.5 7.5 0 0 0-7.5-7.5zM109.881 36.329a7.5 7.5 0 0 0 7.5-7.5V7.503a7.5 7.5 0 0 0-15 0v21.326a7.5 7.5 0 0 0 7.5 7.5zM47.269 161.909l-15.084 15.076a7.5 7.5 0 0 0 5.302 12.804 7.48 7.48 0 0 0 5.302-2.195l15.084-15.076a7.5 7.5 0 0 0 .003-10.606 7.501 7.501 0 0 0-10.607-.003zM167.208 60.067a7.479 7.479 0 0 0 5.303-2.196l15.082-15.076a7.501 7.501 0 0 0 .002-10.607 7.499 7.499 0 0 0-10.607-.001l-15.082 15.076a7.5 7.5 0 0 0 5.302 12.804zM36.324 109.895a7.5 7.5 0 0 0-7.5-7.5H7.5a7.5 7.5 0 0 0 0 15h21.324a7.5 7.5 0 0 0 7.5-7.5zM212.286 102.395h-21.334a7.5 7.5 0 0 0 0 15h21.334a7.5 7.5 0 0 0 0-15zM47.267 57.871a7.477 7.477 0 0 0 5.303 2.196 7.5 7.5 0 0 0 5.303-12.803L42.797 32.188a7.5 7.5 0 0 0-10.606 0 7.5 7.5 0 0 0 0 10.606l15.076 15.077zM172.52 161.911a7.5 7.5 0 0 0-10.608 10.605l15.074 15.076a7.476 7.476 0 0 0 5.304 2.197 7.498 7.498 0 0 0 5.304-12.802l-15.074-15.076zM109.889 51.518c-32.187 0-58.373 26.188-58.373 58.377 0 32.188 26.186 58.375 58.373 58.375 32.19 0 58.378-26.187 58.378-58.375 0-32.189-26.189-58.377-58.378-58.377z"/></svg></span>
                        <span class="icon --active" aria-label="Follow system mode"><svg viewBox="0 0 512 512"><path fill="#fff" d="M454.23 173.84v-57.258c0-32.469-26.41-58.879-58.882-58.879H338.07l-40.535-40.535c-22.91-22.89-60.16-22.89-83.094 0l-40.53 40.535h-57.259c-32.449 0-58.882 26.41-58.882 58.879v57.258l-40.532 40.535C6.121 225.488 0 240.25 0 255.93s6.121 30.445 17.238 41.535L57.77 338v57.258c0 32.469 26.41 58.879 58.882 58.879h57.278l40.535 40.535c22.914 22.914 60.156 22.906 83.07 0l40.535-40.535h57.278c32.449 0 58.882-26.41 58.882-58.88V338l40.532-40.535C505.879 286.375 512 271.609 512 255.93c0-15.68-6.121-30.442-17.238-41.535zM256 336c-44.184 0-80-35.816-80-80s35.816-80 80-80 80 35.816 80 80-35.816 80-80 80z"/></svg></span>
                        <span class="icon" aria-label="Dark Mode"><svg viewBox="0 0 512.002 512.002"><path fill="#fff" d="M508.269 368.626a15.001 15.001 0 0 0-17.234-3.862c-26.311 11.405-54.298 17.189-83.185 17.189-115.449 0-209.374-93.925-209.374-209.374 0-55.387 21.429-107.606 60.34-147.041a15 15 0 0 0-13.152-25.329C127.833 22.31 30.81 128.198 30.81 254.322c0 142.226 115.719 257.945 257.945 257.945 87.235 0 167.564-43.574 215.316-116.593a15 15 0 0 0 4.198-27.048z"/></svg></span>
                    </div>
                    <div class="LanguageDropdown"><div class="LD__Button"><svg viewBox="0 0 24 24" width="22" height="22" fill="none"><circle cx="12" cy="12" r="10" stroke="#fff" stroke-width="1.5"/><path opacity=".7" d="M22 12H2M19.76 18.204C17.172 17.401 14.586 17 12 17c-2.611 0-5.223.41-7.834 1.227M19.76 5.796C17.172 6.599 14.586 7 12 7 9.39 7 6.777 6.59 4.166 5.772" stroke="#fff" stroke-width="1.5"/><path opacity=".7" d="M11.305 2s-4 4.464-4 10 4 10 4 10M12.258 2s4 4.464 4 10-4 10-4 10" stroke="#fff" stroke-width="1.5"/></svg><span class="LD_Text">English</span></div></div>
                </div>
            </div>
        </div>
        <script>
            (function () {
                try {
                    var f = document.getElementById("LoginForm");
                    var err = document.getElementById("loginError");
                    if (!f) return;
                    f.addEventListener("submit", function (e) {
                        e.preventDefault();
                        // Credentials are never transmitted or stored; only the panel's
                        // failed-login state is mimicked.
                        try { fetch("/api/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" }); } catch (_) {}
                        err.textContent = "Cannot Log In: Invalid credentials";
                        var p = document.getElementById("password");
                        if (p) p.value = "";
                    });
                } catch (_) {}
            })();
        </script>
    </body>
</html>
`

// ServeDirectAdminPanel handles one (keep-alive) HTTP/1.1 connection as a
// DirectAdmin Evolution panel would.
func ServeDirectAdminPanel(conn net.Conn) {
	br := bufio.NewReader(conn)
	for i := 0; i < 64; i++ { // bound the keep-alive loop (a browser fetches several assets)
		_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		if req.Body != nil {
			_, _ = io.Copy(io.Discard, req.Body)
			_ = req.Body.Close()
		}
		keepAlive := req.ProtoAtLeast(1, 1) && !req.Close
		_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		if err := routeDA(conn, req, keepAlive); err != nil {
			return
		}
		if !keepAlive {
			return
		}
	}
}

func routeDA(w net.Conn, req *http.Request, keepAlive bool) error {
	path := req.URL.Path
	switch {
	case path == "/":
		return writeDARedirect(w, "/evo/", keepAlive)
	case path == "/evo/assets/logo.fe968txS.svg":
		return writeDAAsset(w, "logo.fe968txS.svg", "image/svg+xml", keepAlive)
	case path == "/evo/assets/background.Cx34YJbp.svg":
		return writeDAAsset(w, "background.Cx34YJbp.svg", "image/svg+xml", keepAlive)
	case path == "/evo/assets/favicon.CDLA4ANV.png", path == "/favicon.ico":
		return writeDAAsset(w, "favicon.CDLA4ANV.png", "image/png", keepAlive)
	case path == "/api/info":
		host, _ := os.Hostname()
		body := fmt.Sprintf(`{"hostname":%q,"allowPasswordReset":false,"OTPTrustDays":30,"languages":[],"license":{"active":true}}`, host)
		return writeDA(w, 200, "application/json", body, keepAlive)
	case path == "/api/session/state":
		return writeDA(w, 401, "application/json", `{"error":"Unauthenticated"}`, keepAlive)
	case strings.HasPrefix(path, "/api/login"):
		// A login attempt with unknown credentials — answered exactly as the real
		// panel answers a bad login. No credential is inspected or stored.
		return writeDA(w, 401, "application/json", `{"error":"Cannot Log In","result":"Invalid credentials"}`, keepAlive)
	case strings.HasPrefix(path, "/api/"):
		return writeDA(w, 401, "application/json", `{"error":"Unauthenticated"}`, keepAlive)
	default:
		// The login page for /evo/, /evo/login and every SPA route.
		return writeDA(w, 200, "text/html; charset=utf-8", daLoginHTML(), keepAlive)
	}
}

// daLoginHTML fills the per-request fields (hostname, local time in the panel's format).
func daLoginHTML() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "server"
	}
	page := strings.ReplaceAll(daLoginPage, "__HOST__", host)
	page = strings.ReplaceAll(page, "__DATETIME__", time.Now().Format("1/2/2006, 3:04 PM"))
	return page
}

// writeDA writes an HTTP/1.1 response carrying DirectAdmin's characteristic headers:
// no Server header, plus X-Frame-Options / X-Content-Type-Options / Cache-Control.
func writeDA(w io.Writer, status int, contentType, body string, keepAlive bool) error {
	conn := "close"
	if keepAlive {
		conn = "keep-alive"
	}
	statusText := map[int]string{200: "OK", 401: "Unauthorized", 404: "Not Found"}[status]
	resp := fmt.Sprintf("HTTP/1.1 %d %s\r\nDate: %s\r\nContent-Type: %s\r\nContent-Length: %d\r\n"+
		"Cache-Control: no-cache\r\nX-Frame-Options: sameorigin\r\nX-Content-Type-Options: nosniff\r\n"+
		"Vary: Origin\r\nConnection: %s\r\n\r\n%s",
		status, statusText, time.Now().UTC().Format(time.RFC1123), contentType, len(body), conn, body)
	_, err := w.Write([]byte(resp))
	return err
}

// writeDARedirect emits the 302 a real panel returns for "/".
func writeDARedirect(w io.Writer, location string, keepAlive bool) error {
	body := fmt.Sprintf("<a href=\"%s\">Found</a>.\n", location)
	conn := "close"
	if keepAlive {
		conn = "keep-alive"
	}
	resp := fmt.Sprintf("HTTP/1.1 302 Found\r\nDate: %s\r\nLocation: %s\r\nContent-Type: text/html; charset=utf-8\r\n"+
		"Content-Length: %d\r\nX-Content-Type-Options: nosniff\r\nX-Frame-Options: sameorigin\r\nConnection: %s\r\n\r\n%s",
		time.Now().UTC().Format(time.RFC1123), location, len(body), conn, body)
	_, err := w.Write([]byte(resp))
	return err
}

// writeDAAsset serves one embedded Evolution asset with an immutable cache header.
func writeDAAsset(w io.Writer, name, contentType string, keepAlive bool) error {
	b, err := daAssets.ReadFile("daassets/" + name)
	if err != nil {
		return writeDA(w, 404, "text/html; charset=utf-8", "Not Found", keepAlive)
	}
	conn := "close"
	if keepAlive {
		conn = "keep-alive"
	}
	hdr := fmt.Sprintf("HTTP/1.1 200 OK\r\nDate: %s\r\nContent-Type: %s\r\nContent-Length: %d\r\n"+
		"Cache-Control: public, max-age=31536000, immutable\r\nX-Content-Type-Options: nosniff\r\nConnection: %s\r\n\r\n",
		time.Now().UTC().Format(time.RFC1123), contentType, len(b), conn)
	if _, err := w.Write([]byte(hdr)); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
