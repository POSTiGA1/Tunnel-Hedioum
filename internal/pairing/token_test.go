package pairing

import "testing"

// TestRoundTrip: a token encodes and decodes back to the same fields.
func TestRoundTrip(t *testing.T) {
	in := Token{
		ExitIP:  "95.179.148.154",
		AuthKey: "aabbccddeeff00112233445566778899",
		Persona: "cpanel",
		SNI:     "vpn.example.com",
		Endpoints: map[string]int{
			"ssh": 2200, "tls": 443, "cpanel": 2083, "mysql": 3306,
		},
	}
	s := Encode(in)
	got, isV2, err := Decode(s)
	if err != nil || !isV2 {
		t.Fatalf("decode: isV2=%v err=%v", isV2, err)
	}
	if got.Version != Version || got.ExitIP != in.ExitIP || got.AuthKey != in.AuthKey ||
		got.Persona != in.Persona || got.SNI != in.SNI || len(got.Endpoints) != len(in.Endpoints) {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	for k, v := range in.Endpoints {
		if got.Endpoints[k] != v {
			t.Fatalf("endpoint %s: got %d want %d", k, got.Endpoints[k], v)
		}
	}
}

// TestLegacyHex: a bare v1 hex token is recognized and not treated as v2.
func TestLegacyHex(t *testing.T) {
	hex := "0189e772bfe63d815f026b8f76d292ea"
	if !IsLegacyHex(hex) {
		t.Fatal("should be legacy hex")
	}
	tok, isV2, err := Decode(hex)
	if err != nil || isV2 || tok != nil {
		t.Fatalf("legacy hex should decode to (nil,false,nil): %v %v %v", tok, isV2, err)
	}
}

// TestRejectGarbage: an unparseable string errors.
func TestRejectGarbage(t *testing.T) {
	if _, _, err := Decode("!!!not-a-token!!!"); err == nil {
		t.Fatal("garbage should error")
	}
}

// TestValidation: decode rejects a token with a bad auth key, no IP, or no endpoints.
func TestValidation(t *testing.T) {
	for _, bad := range []Token{
		{ExitIP: "1.2.3.4", AuthKey: "short", Endpoints: map[string]int{"tls": 443}},
		{ExitIP: "", AuthKey: "aabbccddeeff00112233445566778899", Endpoints: map[string]int{"tls": 443}},
		{ExitIP: "1.2.3.4", AuthKey: "aabbccddeeff00112233445566778899", Endpoints: map[string]int{}},
		{ExitIP: "1.2.3.4", AuthKey: "aabbccddeeff00112233445566778899", Endpoints: map[string]int{"tls": 0}},
	} {
		if _, _, err := Decode(Encode(bad)); err == nil {
			t.Fatalf("should reject: %+v", bad)
		}
	}
}
