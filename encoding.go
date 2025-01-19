package zmqwebproxy

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
)

// Base64RFCString (un)marshals bytes to and from RFC Base64
type Base64RFCString []byte

// String hex-encodes the bytes
func (b Base64RFCString) String() string {
	base64Str := base64.StdEncoding.EncodeToString(b)
	return base64Str
}

// MarshalJSON encodes bytes to RFC Base64
func (b Base64RFCString) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

// UnmarshalJSON decodes RFC Base64 into bytes
func (b *Base64RFCString) UnmarshalJSON(data []byte) error {
	var base64Str string
	if err := json.Unmarshal(data, &base64Str); err != nil {
		return err
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Str) // includes padding
	if err != nil {
		return err
	}

	*b = Base64RFCString(decoded)
	return nil
}

// HexString (un)marshals bytes to and from Hex
type HexString []byte

// String hex-encodes the bytes
func (h HexString) String() string {
	hexStr := hex.EncodeToString(h)
	return hexStr
}

// MarshalJSON encodes bytes to Hex
func (h HexString) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

// UnmarshalJSON decodes Hex into bytes
func (h *HexString) UnmarshalJSON(data []byte) error {
	var hexStr string
	if err := json.Unmarshal(data, &hexStr); err != nil {
		return err
	}
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		return err
	}
	*h = decoded
	return nil
}
