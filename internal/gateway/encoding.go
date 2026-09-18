package gateway

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// requestBody bounds both wire bytes and expanded bytes. Official Codex OAuth
// clients send zstd; rejecting every compressed request breaks that client even
// when the same configuration works with an API-key test fixture.
func requestBody(r *http.Request) ([]byte, int, error) {
	defer r.Body.Close()
	encoding := strings.ToLower(strings.TrimSpace(strings.Join(r.Header.Values("Content-Encoding"), ",")))
	switch encoding {
	case "", "identity", "gzip", "zstd":
	default:
		return nil, http.StatusUnsupportedMediaType, errors.New("支持未压缩 JSON、gzip 或 zstd；不支持多层编码")
	}
	wire, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if len(wire) > maxRequestBytes {
		return nil, http.StatusRequestEntityTooLarge, errors.New("压缩前请求体超过 16 MiB 限制")
	}
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("无法完整读取请求体")
	}
	var reader io.Reader = bytes.NewReader(wire)
	switch encoding {
	case "gzip":
		decoder, err := gzip.NewReader(reader)
		if err != nil {
			return nil, http.StatusBadRequest, errors.New("gzip 请求体损坏或不完整")
		}
		defer decoder.Close()
		reader = decoder
	case "zstd":
		decoder, err := zstd.NewReader(reader, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(maxRequestBytes), zstd.WithDecoderMaxWindow(maxRequestBytes))
		if err != nil {
			return nil, http.StatusBadRequest, errors.New("zstd 请求体损坏或不完整")
		}
		defer decoder.Close()
		reader = decoder
	default:
		return wire, 0, nil
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxRequestBytes+1))
	if len(body) > maxRequestBytes || errors.Is(err, zstd.ErrDecoderSizeExceeded) || errors.Is(err, zstd.ErrWindowSizeExceeded) {
		return nil, http.StatusRequestEntityTooLarge, errors.New("解压后的请求体或 zstd 窗口超过 16 MiB 限制")
	}
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("压缩请求体损坏或不完整")
	}
	return body, 0, nil
}
