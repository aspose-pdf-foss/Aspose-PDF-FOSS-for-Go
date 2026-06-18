// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/sha256"
	"encoding/asn1"
	"testing"
)

// TestBuildTimestampRequest checks the RFC 3161 TimeStampReq is well-formed:
// version 1, a SHA-256 imprint of the signature, and certReq set. No network.
func TestBuildTimestampRequest(t *testing.T) {
	sig := []byte("a signature value to be timestamped")
	der, err := buildTimestampRequest(sig)
	if err != nil {
		t.Fatalf("buildTimestampRequest: %v", err)
	}
	var req timeStampReq
	if _, err := asn1.Unmarshal(der, &req); err != nil {
		t.Fatalf("the request does not parse as a TimeStampReq: %v", err)
	}
	if req.Version != 1 {
		t.Errorf("version = %d, want 1", req.Version)
	}
	if !req.MessageImprint.HashAlgorithm.Algorithm.Equal(oidDigestSHA256) {
		t.Errorf("hash algorithm = %v, want SHA-256", req.MessageImprint.HashAlgorithm.Algorithm)
	}
	want := sha256.Sum256(sig)
	if !bytes.Equal(req.MessageImprint.HashedMessage, want[:]) {
		t.Error("imprint is not SHA-256 of the signature")
	}
	if !req.CertReq {
		t.Error("certReq = false, want true (ask the TSA to embed its certificate)")
	}
}

// TestMarshalTimestampUnsignedAttr checks the token is wrapped as a [1]
// IMPLICIT SET carrying the signature-time-stamp attribute. No network.
func TestMarshalTimestampUnsignedAttr(t *testing.T) {
	token := []byte{0x30, 0x03, 0x02, 0x01, 0x00} // a tiny placeholder DER value
	out, err := marshalTimestampUnsignedAttr(token)
	if err != nil {
		t.Fatalf("marshalTimestampUnsignedAttr: %v", err)
	}
	if len(out) == 0 || out[0] != 0xA1 { // [1] IMPLICIT, constructed
		t.Fatalf("unsigned attrs not tagged [1] (first byte 0x%02X)", out[0])
	}
	var raw asn1.RawValue
	if _, err := asn1.Unmarshal(out, &raw); err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	var attr attribute
	if _, err := asn1.Unmarshal(raw.Bytes, &attr); err != nil {
		t.Fatalf("parse attribute: %v", err)
	}
	if !attr.Type.Equal(oidAttrSignatureTimeStamp) {
		t.Errorf("attr type = %v, want signature-time-stamp", attr.Type)
	}
}
