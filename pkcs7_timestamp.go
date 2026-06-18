// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/sha256"
	"encoding/asn1"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RFC 3161 trusted timestamps. A timestamp is fetched from a Time-Stamp
// Authority over the signature value and embedded as the signature-time-stamp
// unsigned attribute, so the signing time is anchored to a trusted clock
// rather than the signer's computer.

// oidAttrSignatureTimeStamp is the CMS signature-time-stamp attribute
// (RFC 3161 Appendix A / RFC 5652).
var oidAttrSignatureTimeStamp = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}

// messageImprint and timeStampReq model the RFC 3161 TimeStampReq.
type messageImprint struct {
	HashAlgorithm algorithmIdentifier
	HashedMessage []byte
}

type timeStampReq struct {
	Version        int
	MessageImprint messageImprint
	CertReq        bool `asn1:"optional"`
}

// buildTimestampRequest builds a DER TimeStampReq whose imprint is
// SHA-256(signature), asking the TSA to include its certificate.
func buildTimestampRequest(signature []byte) ([]byte, error) {
	h := sha256.Sum256(signature)
	return asn1.Marshal(timeStampReq{
		Version: 1,
		MessageImprint: messageImprint{
			HashAlgorithm: algorithmIdentifier{Algorithm: oidDigestSHA256, Parameters: asn1NULL()},
			HashedMessage: h[:],
		},
		CertReq: true,
	})
}

// requestTimestamp POSTs a TimeStampReq to tsaURL and returns the
// TimeStampToken (a CMS ContentInfo) from the response.
func requestTimestamp(tsaURL string, signature []byte) ([]byte, error) {
	reqDER, err := buildTimestampRequest(signature)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, tsaURL, bytes.NewReader(reqDER))
	if err != nil {
		return nil, fmt.Errorf("timestamp: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/timestamp-query")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("timestamp: request to %s: %w", tsaURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("timestamp: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("timestamp: TSA returned HTTP %d", resp.StatusCode)
	}

	// TimeStampResp ::= SEQUENCE { status PKIStatusInfo, timeStampToken ContentInfo OPTIONAL }
	var tsResp struct {
		Status         asn1.RawValue
		TimeStampToken asn1.RawValue `asn1:"optional"`
	}
	if _, err := asn1.Unmarshal(body, &tsResp); err != nil {
		return nil, fmt.Errorf("timestamp: parse response: %w", err)
	}
	if len(tsResp.TimeStampToken.FullBytes) == 0 {
		return nil, fmt.Errorf("timestamp: TSA response carried no token (status rejected)")
	}
	return tsResp.TimeStampToken.FullBytes, nil
}

// marshalTimestampUnsignedAttr wraps the token in the signature-time-stamp
// unsigned attribute, encoded as a [1] IMPLICIT SET OF Attribute.
func marshalTimestampUnsignedAttr(token []byte) ([]byte, error) {
	attr := attribute{
		Type:   oidAttrSignatureTimeStamp,
		Values: []asn1.RawValue{{FullBytes: token}},
	}
	return marshalImplicitSetTag(1, []attribute{attr})
}
