package transcoder

import (
	"encoding/binary"
	"fmt"
	"io"
)

// EncodeGRPCFrame wraps payload bytes in a 5-byte gRPC wire frame header ([0x00][4-byte big-endian length] + payload).
func EncodeGRPCFrame(payload []byte) []byte {
	length := uint32(len(payload))
	frame := make([]byte, 5+len(payload))
	frame[0] = 0x00 // Compression flag (0 = uncompressed)
	binary.BigEndian.PutUint32(frame[1:5], length)
	copy(frame[5:], payload)
	return frame
}

// DecodeGRPCFrame extracts the binary payload from a 5-byte gRPC wire frame.
func DecodeGRPCFrame(reader io.Reader) ([]byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, fmt.Errorf("failed to read gRPC frame header: %w", err)
	}

	// header[0] is compressed flag
	length := binary.BigEndian.Uint32(header[1:5])
	if length == 0 {
		return []byte{}, nil
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, fmt.Errorf("failed to read gRPC frame payload (%d bytes): %w", length, err)
	}

	return payload, nil
}
