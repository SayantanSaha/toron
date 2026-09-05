package transcoder

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// DefaultMaxGRPCFrameSize defines the default upper bound for incoming gRPC wire frame payloads (4 MB).
const DefaultMaxGRPCFrameSize uint32 = 4 * 1024 * 1024

// ErrFrameTooLarge indicates that a gRPC wire frame payload exceeded the allowed maximum size.
var ErrFrameTooLarge = errors.New("transcoder: gRPC wire frame payload exceeds maximum allowed size")

// EncodeGRPCFrame wraps payload bytes in a 5-byte gRPC wire frame header ([0x00][4-byte big-endian length] + payload).
func EncodeGRPCFrame(payload []byte) []byte {
	length := uint32(len(payload))
	frame := make([]byte, 5+len(payload))
	frame[0] = 0x00 // Compression flag (0 = uncompressed)
	binary.BigEndian.PutUint32(frame[1:5], length)
	copy(frame[5:], payload)
	return frame
}

// DecodeGRPCFrame extracts the binary payload from a 5-byte gRPC wire frame,
// enforcing DefaultMaxGRPCFrameSize before allocating memory buffers.
func DecodeGRPCFrame(reader io.Reader) ([]byte, error) {
	return DecodeGRPCFrameWithLimit(reader, DefaultMaxGRPCFrameSize)
}

// DecodeGRPCFrameWithLimit extracts the binary payload from a 5-byte gRPC wire frame,
// enforcing a caller-specified maximum payload limit before allocating memory buffers.
func DecodeGRPCFrameWithLimit(reader io.Reader, maxFrameSize uint32) ([]byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, fmt.Errorf("failed to read gRPC frame header: %w", err)
	}

	// header[0] is compressed flag
	length := binary.BigEndian.Uint32(header[1:5])
	if maxFrameSize > 0 && length > maxFrameSize {
		return nil, fmt.Errorf("%w: frame length %d exceeds maximum %d", ErrFrameTooLarge, length, maxFrameSize)
	}

	if length == 0 {
		return []byte{}, nil
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, fmt.Errorf("failed to read gRPC frame payload (%d bytes): %w", length, err)
	}

	return payload, nil
}
