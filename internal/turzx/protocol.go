package turzx

import (
	"bytes"
	"crypto/cipher"
	"crypto/des"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	"time"
)

const (
	cmdSync    = 10
	cmdRestart = 11
	cmdJPEG    = 101

	maxImageSize = 1 << 20
)

var desKey = []byte("slv3tuzx")

// packet builds the 512-byte control part followed by payload. The timestamp is
// milliseconds since local midnight; the device echoes it in its response.
func packet(command byte, payload []byte, now time.Time) ([]byte, uint32) {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	timestamp := uint32(now.Sub(midnight).Milliseconds())
	header := make([]byte, 504)
	header[0] = command
	header[2], header[3] = 0x1A, 0x6D
	binary.LittleEndian.PutUint32(header[4:], timestamp)
	binary.BigEndian.PutUint32(header[8:], uint32(len(payload)))

	block, err := des.NewCipher(desKey)
	if err != nil {
		panic(err) // The key length is fixed.
	}
	p := make([]byte, 512+len(payload))
	cipher.NewCBCEncrypter(block, desKey).CryptBlocks(p[:504], header)
	p[510], p[511] = 0xA1, 0x1A
	copy(p[512:], payload)
	return p, timestamp
}

func checkResponse(command byte, timestamp uint32, response []byte) error {
	if len(response) < 6 || response[0] != command || response[1] != 0xC8 ||
		binary.LittleEndian.Uint32(response[2:]) != timestamp {
		return fmt.Errorf("unexpected TURZX response to command %d: % X", command, response[:min(len(response), 16)])
	}
	return nil
}

func validateJPEG(data []byte) error {
	if len(data) == 0 || len(data) > maxImageSize {
		return fmt.Errorf("JPEG must be 1..%d bytes, got %d", maxImageSize, len(data))
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode JPEG header: %w", err)
	}
	if config.Width != 462 || config.Height != 1920 {
		return fmt.Errorf("JPEG must be 462x1920, got %dx%d", config.Width, config.Height)
	}
	return nil
}
