// Package contentencoding unwraps a response body whose Content-Encoding
// the HTTP transport left in place because the request set Accept-Encoding.
package contentencoding

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// ErrUnsupportedEncoding reports a Content-Encoding this package cannot read.
var ErrUnsupportedEncoding = errors.New("unsupported content encoding")

type body struct {
	io.Reader
	closers []io.Closer
}

// Close releases every decoder the body read through, reporting the first failure.
func (b body) Close() error {
	var err error
	for _, closer := range b.closers {
		if closeErr := closer.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

// Decode replaces the response body with the decoded bytes when
// Content-Encoding is set. An identity or empty encoding is left alone.
func Decode(response *http.Response) error {
	if response == nil || response.Body == nil {
		return nil
	}
	encodings := encodingsOf(response.Header.Get("Content-Encoding"))
	if len(encodings) == 0 {
		return nil
	}
	reader := io.Reader(response.Body)
	var closers []io.Closer
	for i := len(encodings) - 1; i >= 0; i-- {
		decoded, closer, err := openDecoder(encodings[i], reader)
		if err != nil {
			_ = (body{closers: closers}).Close()
			return err
		}
		reader = decoded
		if closer != nil {
			closers = append([]io.Closer{closer}, closers...)
		}
	}
	closers = append(closers, response.Body)
	response.Body = body{Reader: reader, closers: closers}
	response.Header.Del("Content-Encoding")
	response.ContentLength = -1
	response.Uncompressed = true
	return nil
}

func encodingsOf(header string) []string {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	var encodings []string
	for _, part := range strings.Split(header, ",") {
		name := strings.TrimSpace(part)
		if name == "" || strings.EqualFold(name, "identity") {
			continue
		}
		encodings = append(encodings, name)
	}
	return encodings
}

func openDecoder(name string, reader io.Reader) (io.Reader, io.Closer, error) {
	switch strings.ToLower(name) {
	case "gzip":
		decoded, err := gzip.NewReader(reader)
		if err != nil {
			return nil, nil, err
		}
		return decoded, decoded, nil
	case "deflate":
		return deflateReader(reader)
	case "br":
		return brotli.NewReader(reader), nil, nil
	case "zstd":
		decoded, err := zstd.NewReader(reader)
		if err != nil {
			return nil, nil, err
		}
		return decoded, decoded.IOReadCloser(), nil
	default:
		return nil, nil, fmt.Errorf("%w: %s", ErrUnsupportedEncoding, name)
	}
}

func deflateReader(reader io.Reader) (io.Reader, io.Closer, error) {
	buffered := bufio.NewReader(reader)
	header, err := buffered.Peek(2)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, nil, err
	}
	if zlibHeader(header) {
		decoded, zlibErr := zlib.NewReader(buffered)
		if zlibErr != nil {
			return nil, nil, zlibErr
		}
		return decoded, decoded, nil
	}
	decoded := flate.NewReader(buffered)
	return decoded, decoded, nil
}

func zlibHeader(header []byte) bool {
	if len(header) < 2 || header[0]&0x0f != 8 {
		return false
	}
	return (int(header[0])*256+int(header[1]))%31 == 0
}
