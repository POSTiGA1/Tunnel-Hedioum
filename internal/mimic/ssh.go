package mimic

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultSSHBanner = "SSH-2.0-OpenSSH_8.9p1 Ubuntu-3ubuntu0.10\r\n"
	maxPaddingLength = 64
	minPaddingLength = 16
	handshakeTimeout = 5 * time.Second
)

// GetDynamicSSHBanner extracts local SSH version to remain indistinguishable from real OS
func GetDynamicSSHBanner() string {
	cmd := exec.Command("ssh", "-V")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil && stderr.Len() == 0 {
		return defaultSSHBanner
	}

	output := strings.TrimSpace(stderr.String())
	parts := strings.Split(output, ",")
	if len(parts) > 0 {
		return fmt.Sprintf("SSH-2.0-%s\r\n", strings.TrimSpace(parts[0]))
	}

	return defaultSSHBanner
}

// readBanner safely reads strictly up to the newline character.
// This prevents TCP buffer over-reading which would otherwise consume the obfuscated payload.
func readBanner(conn net.Conn) (string, error) {
	var banner []byte
	buf := make([]byte, 1)

	// Read byte-by-byte up to a sensible maximum length to prevent infinite loops
	for i := 0; i < 255; i++ {
		_, err := conn.Read(buf)
		if err != nil {
			return "", err
		}
		banner = append(banner, buf[0])
		if buf[0] == '\n' {
			break
		}
	}
	return string(banner), nil
}

// PerformClientHandshake sends client banner, reads server banner securely, and dispatches payload.
func PerformClientHandshake(conn net.Conn, token string, targetAddr string) error {
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return err
	}
	defer conn.SetDeadline(time.Time{})

	// 1. Send Client Banner
	banner := GetDynamicSSHBanner()
	if _, err := conn.Write([]byte(banner)); err != nil {
		return fmt.Errorf("failed to write client banner: %w", err)
	}

	// 2. Safely read Server Banner without over-consuming buffer
	if _, err := readBanner(conn); err != nil {
		return fmt.Errorf("failed to read server banner: %w", err)
	}

	// 3. Construct Obfuscated Metadata Payload: [TokenLen] [Token] [TargetLen] [Target]
	payload := new(bytes.Buffer)
	payload.WriteByte(byte(len(token)))
	payload.WriteString(token)

	targetLenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(targetLenBytes, uint16(len(targetAddr)))
	payload.Write(targetLenBytes)
	payload.WriteString(targetAddr)

	// 4. Wrap Payload in RFC 4253 SSH Binary Packet Format
	paddingLen := generateRandomInt(minPaddingLength, maxPaddingLength)
	packetLen := uint32(1 + payload.Len() + paddingLen)

	packet := new(bytes.Buffer)
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, packetLen)

	packet.Write(lengthBytes)
	packet.WriteByte(byte(paddingLen))
	packet.Write(payload.Bytes())

	// Append cryptographically secure noise
	randomPadding := make([]byte, paddingLen)
	if _, err := rand.Read(randomPadding); err != nil {
		return errors.New("failed to generate secure padding noise")
	}
	packet.Write(randomPadding)

	if _, err := conn.Write(packet.Bytes()); err != nil {
		return fmt.Errorf("failed to send obfuscated metadata packet: %w", err)
	}

	return nil
}

// PerformServerHandshake verifies client authenticity and extracts metadata securely.
func PerformServerHandshake(conn net.Conn, expectedToken string) (string, error) {
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return "", err
	}
	defer conn.SetDeadline(time.Time{})

	// 1. Send Server Banner
	banner := GetDynamicSSHBanner()
	if _, err := conn.Write([]byte(banner)); err != nil {
		return "", fmt.Errorf("failed to write server banner: %w", err)
	}

	// 2. Safely read Client Banner without over-consuming buffer
	clientBanner, err := readBanner(conn)
	if err != nil {
		return "", fmt.Errorf("failed to read client banner: %w", err)
	}

	if !strings.HasPrefix(clientBanner, "SSH-2.0") {
		return "", errors.New("invalid protocol banner signature")
	}

	// 3. Read Obfuscated Packet Header
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", errors.New("failed to read metadata packet header")
	}

	packetLen := binary.BigEndian.Uint32(header[0:4])
	paddingLen := int(header[4])

	payloadLen := int(packetLen) - 1 - paddingLen
	if payloadLen <= 0 || payloadLen > 1024 {
		return "", errors.New("malformed obfuscated packet dimensions")
	}

	// 4. Read Payload + Padding
	bodyBuf := make([]byte, payloadLen+paddingLen)
	if _, err := io.ReadFull(conn, bodyBuf); err != nil {
		return "", errors.New("failed to read obfuscated payload body")
	}

	// 5. Validate Token
	payloadData := bodyBuf[:payloadLen]
	tokenLen := int(payloadData[0])

	if tokenLen+1 > payloadLen {
		return "", errors.New("payload bounds exceeded reading token")
	}

	receivedToken := string(payloadData[1 : 1+tokenLen])
	if receivedToken != expectedToken {
		return "", errors.New("authentication token mismatch - rogue scanner dropped")
	}

	// 6. Extract Target
	targetLenOffset := 1 + tokenLen
	if targetLenOffset+2 > payloadLen {
		return "", errors.New("payload bounds exceeded reading target length")
	}

	targetLen := int(binary.BigEndian.Uint16(payloadData[targetLenOffset : targetLenOffset+2]))
	targetStrOffset := targetLenOffset + 2

	if targetStrOffset+targetLen > payloadLen {
		return "", errors.New("payload bounds exceeded reading target string")
	}

	targetAddr := string(payloadData[targetStrOffset : targetStrOffset+targetLen])

	return targetAddr, nil
}

func generateRandomInt(min, max int) int {
	b := make([]byte, 1)
	_, _ = rand.Read(b)
	return min + int(b[0])%(max-min+1)
}
