package mimic

import (
	"fmt"
	"net"
	"time"
)

// apacheDefaultPage mirrors the recognizable Apache2 Ubuntu "Default Page" so a
// probe or a person hitting the port sees exactly what a freshly-installed Apache
// on Ubuntu serves — nothing tunnel-specific. Trimmed but faithful to the real
// page's title, banner, and wording.
const apacheDefaultPage = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" lang="en">
  <head>
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
    <title>Apache2 Ubuntu Default Page: It works</title>
    <style type="text/css">
      * { margin: 0px 0px 0px 0px; padding: 0px 0px 0px 0px; }
      body, html { padding: 3px 3px 3px 3px; background-color: #D8DBE2; font-family: Verdana, sans-serif; font-size: 11pt; text-align: center; }
      div.main_page { position: relative; display: table; width: 800px; margin-bottom: 3px; margin-left: auto; margin-right: auto; padding: 0px; border-width: 2px; border-color: #212738; border-style: solid; background-color: #FFFFFF; text-align: center; }
      div.page_header { height: 99px; width: 100%; background-color: #F5F6F7; }
      div.page_header span { margin: 15px 0px 0px 50px; font-size: 180%; font-weight: bold; }
      div.table_of_contents { clear: left; min-width: 200px; margin: 3px 3px 3px 3px; background-color: #FFFFFF; text-align: left; }
      div.content_section { margin: 3px 3px 3px 3px; background-color: #FFFFFF; text-align: left; }
      div.content_section_text { padding: 4px 8px 4px 8px; color: #000000; font-size: 100%; }
      div.content_section_text a { color: #000000; font-weight: bold; }
      div.validator {}
    </style>
  </head>
  <body>
    <div class="main_page">
      <div class="page_header floating_element">
        <span class="floating_element">Apache2 Ubuntu Default Page</span>
      </div>
      <div class="content_section floating_element">
        <div class="content_section_text">
          <p><span style="font-size: 130%; font-weight: bold;">It works!</span></p>
          <p>This is the default welcome page used to test the correct operation of the Apache2 server after installation on Ubuntu systems. It is based on the equivalent page on Debian, from which the Ubuntu Apache packaging is derived. If you can read this page, it means that the Apache HTTP server installed at this site is working properly. You should <b>replace this file</b> (located at <tt>/var/www/html/index.html</tt>) before continuing to operate your HTTP server.</p>
          <p>If you are a normal user of this web site and don't know what this page is about, this probably means that the site is currently unavailable due to maintenance. If the problem persists, please contact the site's administrator.</p>
        </div>
      </div>
    </div>
    <div class="validator"></div>
  </body>
</html>`

// daWebDefault is exactly what a real DirectAdmin box serves on its web ports
// (:80/:443) when hit without the panel port — a 47-byte static file. Captured
// verbatim from a live DirectAdmin server.
const daWebDefault = "<html>webserver is functioning normally</html>\n"

// ServeWebDecoy answers any connection with the Apache2 Ubuntu default page, so a
// port reads as an ordinary web host to scanners and IP-reputation checks. Used
// both by the TLS mimic's decoy (inside TLS) and by the plaintext :80 decoy.
func ServeWebDecoy(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _ = conn.Read(make([]byte, 4096)) // consume (part of) the request line/headers
	resp := fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nDate: %s\r\nServer: Apache/2.4.52 (Ubuntu)\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		time.Now().UTC().Format(time.RFC1123), len(apacheDefaultPage), apacheDefaultPage,
	)
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, _ = conn.Write([]byte(resp))
}

// ServeDirectAdminWeb answers with the exact page a DirectAdmin server serves on its
// web ports — "webserver is functioning normally" with an Apache/2 signature — so
// the box reads as an ordinary DirectAdmin hosting server to scanners.
func ServeDirectAdminWeb(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _ = conn.Read(make([]byte, 4096))
	resp := fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nDate: %s\r\nServer: Apache/2\r\nLast-Modified: Mon, 27 Oct 2025 18:36:27 GMT\r\nAccept-Ranges: bytes\r\nContent-Length: %d\r\nVary: User-Agent\r\nContent-Type: text/html\r\nConnection: close\r\n\r\n%s",
		time.Now().UTC().Format(time.RFC1123), len(daWebDefault), daWebDefault,
	)
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, _ = conn.Write([]byte(resp))
}

// WebDecoyFor returns the raw (net.Conn) web decoy for a persona style. Unknown or
// empty styles fall back to the Apache default.
func WebDecoyFor(style string) func(net.Conn) {
	if style == "directadmin" {
		return ServeDirectAdminWeb
	}
	return ServeWebDecoy
}
