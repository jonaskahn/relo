// Package awsauth signs requests with AWS Signature V4 for Bedrock and
// SageMaker, so either vendor accepts what the relay sends.
package awsauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// SignOptions names everything an AWS Signature V4 signing needs: the
// request and body to sign, the credential that signs it, and where and
// when the request goes.
type SignOptions struct {
	Request      *http.Request
	Body         []byte
	AccessKey    string
	SecretKey    string
	SessionToken string
	Region       string
	Service      string
	Now          time.Time
}

// Sign signs an outbound HTTP request using AWS Signature Version 4.
func Sign(opts SignOptions) error {
	req, now := opts.Request, opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	amzDate, dateStamp := sigDates(now)
	payloadHash := sha256Hex(opts.Body)
	stampSigHeaders(req, opts.SessionToken, amzDate, payloadHash)
	signedHeaders, canonicalHeaders := canonicalSigHeaders(req, opts.SessionToken, amzDate, payloadHash)
	canonicalRequest := sigCanonicalRequest(req, canonicalHeaders, signedHeaders, payloadHash)
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, opts.Region, opts.Service)
	stringToSign := sigStringToSign(amzDate, credentialScope, canonicalRequest)
	signingKey := getSignatureKey(opts.SecretKey, dateStamp, opts.Region, opts.Service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		opts.AccessKey, credentialScope, signedHeaders, signature))
	return nil
}

func sigDates(now time.Time) (string, string) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	return now.Format("20060102T150405Z"), now.Format("20060102")
}

func stampSigHeaders(req *http.Request, sessionToken, amzDate, payloadHash string) {
	req.Header.Set("x-amz-date", amzDate)
	if sessionToken != "" {
		req.Header.Set("x-amz-security-token", sessionToken)
	}
	req.Header.Set("x-amz-content-sha256", payloadHash)
	if req.Host == "" && req.URL != nil {
		req.Host = req.URL.Host
	}
}

func canonicalSigHeaders(req *http.Request, sessionToken, amzDate, payloadHash string) (string, string) {
	headersToSign := map[string]string{
		"host":                 req.Host,
		"x-amz-date":           amzDate,
		"x-amz-content-sha256": payloadHash,
	}
	if sessionToken != "" {
		headersToSign["x-amz-security-token"] = sessionToken
	}
	if ct := req.Header.Get("Content-Type"); ct != "" {
		headersToSign["content-type"] = ct
	}
	headerKeys := make([]string, 0, len(headersToSign))
	for k := range headersToSign {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)
	var canonicalHeaders strings.Builder
	for _, k := range headerKeys {
		canonicalHeaders.WriteString(k)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(headersToSign[k]))
		canonicalHeaders.WriteString("\n")
	}
	return strings.Join(headerKeys, ";"), canonicalHeaders.String()
}

func sigCanonicalRequest(req *http.Request, canonicalHeaders, signedHeaders, payloadHash string) string {
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	return strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQueryString(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
}

func sigStringToSign(amzDate, credentialScope, canonicalRequest string) string {
	return strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
}

func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var pairs []string
	for _, k := range keys {
		vList := values[k]
		sort.Strings(vList)
		encodedKey := url.QueryEscape(k)
		for _, v := range vList {
			pairs = append(pairs, encodedKey+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(pairs, "&")
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func getSignatureKey(key, dateStamp, regionName, serviceName string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+key), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(regionName))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

// ReadAndReplaceBody reads the body bytes and restores the request body.
func ReadAndReplaceBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(strings.NewReader(string(body)))
	return body, nil
}
