package mimic

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
)

// Database STARTTLS-family prologues (PostgreSQL, MySQL/MariaDB). Like the mail
// STARTTLS mimics, a short plaintext protocol negotiation upgrades to TLS, after
// which the shared TLS mimic (real TLS + channel-bound token auth) takes over. So a
// passive observer sees a genuine Postgres/MySQL SSL negotiation on the wire, then
// TLS — indistinguishable from a real database that requires SSL — with no domain
// required. Both are the same StartTLSMimic/StartTLSClient, only the prologue differs.

// --- PostgreSQL: client sends SSLRequest, server replies 'S', then TLS. ---
//
// SSLRequest is a fixed 8-byte message: Int32 length=8, Int32 code=80877103. The
// server answers a single byte 'S' (SSL supported) and the TLS handshake follows on
// the same socket. This is exactly what `psql "sslmode=require"` and libpq do.

const pgSSLRequestCode = 80877103 // 0x04D2162F

func serverPostgresPrologue(conn net.Conn) error {
	var buf [8]byte
	if _, err := io.ReadFull(conn, buf[:]); err != nil {
		return err
	}
	if binary.BigEndian.Uint32(buf[0:4]) != 8 || binary.BigEndian.Uint32(buf[4:8]) != pgSSLRequestCode {
		return errNoStartTLS // not a Postgres SSLRequest -> decoy
	}
	_, err := conn.Write([]byte{'S'}) // SSL supported; TLS ClientHello follows
	return err
}

func clientPostgresPrologue(conn net.Conn) error {
	var msg [8]byte
	binary.BigEndian.PutUint32(msg[0:4], 8)
	binary.BigEndian.PutUint32(msg[4:8], pgSSLRequestCode)
	if _, err := conn.Write(msg[:]); err != nil {
		return err
	}
	var resp [1]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return err
	}
	if resp[0] != 'S' {
		return errNoStartTLS
	}
	return nil
}

// --- MySQL/MariaDB: server sends the v10 handshake greeting, client replies with an
// SSL-request packet (CLIENT_SSL set), then TLS. Server-speaks-first. ---

const mysqlClientSSL = 0x00000800 // CLIENT_SSL capability bit

// mysqlServerVersion is the version string advertised in the greeting. A plain,
// current MySQL release string — the most common thing a scanner sees on :3306.
const mysqlServerVersion = "8.0.36"

func serverMySQLPrologue(conn net.Conn) error {
	if _, err := conn.Write(buildMySQLGreeting()); err != nil {
		return err
	}
	// Read the client's SSL-request packet: 4-byte header (3-byte LE length + seq),
	// then exactly length bytes — never over-reading into the TLS ClientHello.
	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return err
	}
	plen := int(hdr[0]) | int(hdr[1])<<8 | int(hdr[2])<<16
	if plen < 4 || plen > 4096 {
		return errNoStartTLS
	}
	payload := make([]byte, plen)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return err
	}
	// Client capability flags are the first 4 bytes (LE). CLIENT_SSL must be set for
	// the client to start TLS next; anything else is not our client / not SSL.
	if binary.LittleEndian.Uint32(payload[0:4])&mysqlClientSSL == 0 {
		return errNoStartTLS
	}
	return nil
}

func clientMySQLPrologue(conn net.Conn) error {
	// Consume the server greeting: header + exactly length payload bytes.
	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return err
	}
	plen := int(hdr[0]) | int(hdr[1])<<8 | int(hdr[2])<<16
	if plen <= 0 || plen > 4096 {
		return errNoStartTLS
	}
	if _, err := io.ReadFull(conn, make([]byte, plen)); err != nil {
		return err
	}
	_, err := conn.Write(buildMySQLSSLRequest()) // TLS ClientHello follows immediately
	return err
}

// buildMySQLGreeting builds a structurally-correct MySQL 8.0 v10 handshake packet
// (protocol 10, version string, random connection id + salt, CLIENT_SSL advertised,
// caching-compatible auth plugin), so a real client negotiates SSL and a passive
// classifier reads the port as MySQL.
func buildMySQLGreeting() []byte {
	var salt1 [8]byte
	var salt2 [12]byte
	var tid [4]byte
	_, _ = rand.Read(salt1[:])
	_, _ = rand.Read(salt2[:])
	_, _ = rand.Read(tid[:])

	p := []byte{0x0a} // protocol version 10
	p = append(p, mysqlServerVersion...)
	p = append(p, 0x00)      // NUL-terminated version
	p = append(p, tid[:]...) // thread/connection id (4 bytes LE)
	p = append(p, salt1[:]...)
	p = append(p, 0x00)                // filler
	p = append(p, 0xff, 0xff)          // capability flags lower (LE 0xFFFF): incl CLIENT_SSL(0x0800), PROTOCOL_41, SECURE_CONNECTION
	p = append(p, 0xff)                // character set (utf8mb4)
	p = append(p, 0x02, 0x00)          // status flags (LE 0x0002: AUTOCOMMIT)
	p = append(p, 0xff, 0x81)          // capability flags upper (LE 0x81FF): incl PLUGIN_AUTH
	p = append(p, 0x15)                // length of auth-plugin-data (21)
	p = append(p, make([]byte, 10)...) // reserved
	p = append(p, salt2[:]...)         // auth-plugin-data part 2 (12 bytes)
	p = append(p, 0x00)                // NUL terminator for the salt
	p = append(p, "mysql_native_password"...)
	p = append(p, 0x00)

	pkt := make([]byte, 4+len(p))
	pkt[0] = byte(len(p))
	pkt[1] = byte(len(p) >> 8)
	pkt[2] = byte(len(p) >> 16)
	pkt[3] = 0 // sequence 0
	copy(pkt[4:], p)
	return pkt
}

// buildMySQLSSLRequest builds the client SSL-request packet (sequence 1): capability
// flags with CLIENT_SSL set, max-packet-size, charset, 23 reserved bytes — after
// which the client immediately begins the TLS handshake.
func buildMySQLSSLRequest() []byte {
	payload := make([]byte, 32)
	// capability flags (LE): LONG_PASSWORD|LONG_FLAG|PROTOCOL_41|SSL|TRANSACTIONS|
	// SECURE_CONNECTION|PLUGIN_AUTH|DEPRECATE_EOF
	binary.LittleEndian.PutUint32(payload[0:4], 0x0108AA05)
	binary.LittleEndian.PutUint32(payload[4:8], 0x01000000) // max packet size 16MB
	payload[8] = 0xff                                       // charset utf8mb4
	// payload[9:32] reserved (already zero)
	pkt := make([]byte, 4+len(payload))
	pkt[0] = byte(len(payload))
	pkt[3] = 1 // sequence 1 (follows the greeting)
	copy(pkt[4:], payload)
	return pkt
}
