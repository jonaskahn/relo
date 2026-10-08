package contentencoding

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDecodeGzip(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatalf("write gzip: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	response := &http.Response{
		Header: http.Header{"Content-Encoding": {"gzip"}},
		Body:   io.NopCloser(bytes.NewReader(compressed.Bytes())),
	}
	if err := Decode(response); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read decoded body: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %s", body)
	}
	if response.Header.Get("Content-Encoding") != "" {
		t.Fatalf("content-encoding = %q, want it removed", response.Header.Get("Content-Encoding"))
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
}

func TestDecodeDeflateFormats(t *testing.T) {
	t.Run("zlib", func(t *testing.T) {
		var compressed bytes.Buffer
		writer := zlib.NewWriter(&compressed)
		if _, err := writer.Write([]byte("zlib")); err != nil {
			t.Fatalf("write zlib: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close zlib: %v", err)
		}
		if got := decodeEncoded(t, "deflate", compressed.Bytes()); got != "zlib" {
			t.Fatalf("body = %q", got)
		}
	})

	t.Run("raw deflate", func(t *testing.T) {
		var compressed bytes.Buffer
		writer, err := flate.NewWriter(&compressed, flate.DefaultCompression)
		if err != nil {
			t.Fatalf("new flate writer: %v", err)
		}
		if _, err := writer.Write([]byte("raw")); err != nil {
			t.Fatalf("write flate: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close flate: %v", err)
		}
		if got := decodeEncoded(t, "deflate", compressed.Bytes()); got != "raw" {
			t.Fatalf("body = %q", got)
		}
	})
}

func decodeEncoded(t *testing.T, encoding string, payload []byte) string {
	t.Helper()
	response := &http.Response{
		Header: http.Header{"Content-Encoding": {encoding}},
		Body:   io.NopCloser(bytes.NewReader(payload)),
	}
	if err := Decode(response); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read decoded body: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
	return string(body)
}

func TestDecodeLeavesIdentityAlone(t *testing.T) {
	response := &http.Response{
		Header: http.Header{},
		Body:   io.NopCloser(strings.NewReader("plain")),
	}
	if err := Decode(response); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "plain" {
		t.Fatalf("body = %q, %v", body, err)
	}
}

func TestDecodeFailsClosed(t *testing.T) {
	t.Run("an unsupported encoding is refused", func(t *testing.T) {
		response := &http.Response{
			Header: http.Header{"Content-Encoding": {"br-bogus"}},
			Body:   io.NopCloser(strings.NewReader("x")),
		}
		err := Decode(response)
		if err == nil || !errors.Is(err, ErrUnsupportedEncoding) {
			t.Fatalf("Decode() error = %v, want ErrUnsupportedEncoding", err)
		}
	})

	t.Run("a corrupt gzip body reports the reader error", func(t *testing.T) {
		response := &http.Response{
			Header: http.Header{"Content-Encoding": {"gzip"}},
			Body:   io.NopCloser(strings.NewReader("not gzip")),
		}
		if err := Decode(response); err == nil {
			t.Fatal("Decode() error = nil, want a gzip reader error")
		}
	})

	t.Run("a corrupt zstd body reports the reader error", func(t *testing.T) {
		response := &http.Response{
			Header: http.Header{"Content-Encoding": {"zstd"}},
			Body:   io.NopCloser(strings.NewReader("not zstd")),
		}
		if err := Decode(response); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if _, err := io.ReadAll(response.Body); err == nil {
			t.Fatal("read the corrupt zstd body: error = nil, want a magic number error")
		}
		_ = response.Body.Close()
	})

	t.Run("a truncated encoding list ends at the empty name", func(t *testing.T) {
		if got := encodingsOf("gzip, , br"); len(got) != 2 || got[0] != "gzip" || got[1] != "br" {
			t.Fatalf("encodingsOf() = %v, want gzip then br", got)
		}
	})

	t.Run("a gzip body that fails mid-stream reports the read error", func(t *testing.T) {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write([]byte("partial")); err != nil {
			t.Fatalf("write gzip: %v", err)
		}
		// No Close: the checksum and the trailer are missing.
		response := &http.Response{
			Header: http.Header{"Content-Encoding": {"gzip"}},
			Body:   io.NopCloser(bytes.NewReader(compressed.Bytes()[:compressed.Len()/2])),
		}
		if err := Decode(response); err == nil {
			t.Fatal("Decode() error = nil, want an unexpected EOF from the header")
		}
	})
}
