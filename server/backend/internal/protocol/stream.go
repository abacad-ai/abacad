package protocol

// Stream frames are the second lane of the device WebSocket. The command/reply
// lane (Command/Reply above) is JSON over *text* messages; the tunnel lane is
// this compact binary framing over *binary* messages. The two never collide —
// ReadPump dispatches purely on the WebSocket message type — so a phone speaking
// only the accessibility protocol and a desktop carrying a raw TCP tunnel share
// one contract with zero changes to either side's existing code.
//
// A tunnel carries arbitrary TCP (ssh, rsync, git, a database client) so the
// server can treat the device as a reachable host. Streams are always opened
// from the SERVER side (an agent-side client dials in); the device only ever
// answers on an id it was given, so there is no device-initiated OPEN to handle.

import (
	"encoding/binary"
	"errors"
)

// StreamFrameType tags a binary tunnel frame.
type StreamFrameType byte

const (
	// StreamOpen asks the device to dial a TCP target. Payload is the target
	// "host:port" (UTF-8). No reply frame — the dial is optimistic; a failure
	// comes back as a StreamClose carrying the error, exactly like a TCP reset
	// surfacing to an SSH ProxyCommand.
	StreamOpen StreamFrameType = 1
	// StreamData carries raw TCP bytes in either direction. Payload is the bytes.
	StreamData StreamFrameType = 2
	// StreamClose tears the stream down. Empty payload is a clean half/full close
	// (EOF); a non-empty payload is an error reason (e.g. "dial tcp: refused").
	StreamClose StreamFrameType = 3
)

// StreamHeaderLen is the fixed prefix: one type byte + a big-endian uint64 id.
const StreamHeaderLen = 1 + 8

// ErrShortStreamFrame means a binary frame was too small to hold the header.
var ErrShortStreamFrame = errors.New("stream frame shorter than header")

// EncodeStreamFrame builds [type][stream id BE][payload]. payload may be nil.
func EncodeStreamFrame(t StreamFrameType, streamID uint64, payload []byte) []byte {
	buf := make([]byte, StreamHeaderLen+len(payload))
	buf[0] = byte(t)
	binary.BigEndian.PutUint64(buf[1:StreamHeaderLen], streamID)
	copy(buf[StreamHeaderLen:], payload)
	return buf
}

// DecodeStreamFrame splits a binary frame. The returned payload aliases buf; a
// caller that keeps it beyond the current read must copy it first.
func DecodeStreamFrame(buf []byte) (t StreamFrameType, streamID uint64, payload []byte, err error) {
	if len(buf) < StreamHeaderLen {
		return 0, 0, nil, ErrShortStreamFrame
	}
	return StreamFrameType(buf[0]), binary.BigEndian.Uint64(buf[1:StreamHeaderLen]), buf[StreamHeaderLen:], nil
}
