// Package httpx holds helpers shared by the HTTP source adapters.
package httpx

import (
	"fmt"
	"io"

	"github.com/PloyBox/get-lyrics/source"
)

// MaxBodyBytes caps how much of an upstream response is read.
const MaxBodyBytes = 8 << 20

// ReadBody reads r up to MaxBodyBytes, failing when the body is larger.
func ReadBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, MaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBodyBytes {
		return nil, fmt.Errorf("%w (%d bytes)", source.ErrBodyTooLarge, MaxBodyBytes)
	}
	return body, nil
}
