// Package protocol implements decoding and encoding of client/server messages of the Locksmith protocol.
package protocol

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog/log"
)

// ServerMessageType encompasses all messages: Client -> Locksmith.
type ServerMessageType byte

const (
	Acquire ServerMessageType = 0
	Release ServerMessageType = 1
)

// ClientMessageType encompasses all messages: Locksmith -> Client.
type ClientMessageType byte

const (
	Acquired ClientMessageType = 0
)

// Errors returned by encoding/decoding functions.
var (
	ErrServerMessageDecode = errors.New("server message decoding error")
	ErrClientMessageDecode = errors.New("client message decoding error")
	ErrServerMessageType   = errors.New("server message type not found")
	ErrClientMessageType   = errors.New("client message type not found")
	ErrLockTagSize         = errors.New("lock tag size does not match actual lock tag size")
	ErrLockTagEncoding     = errors.New("lock tag was not valid UTF8")
)

// ServerMessage models a server-bound message.
type ServerMessage struct {
	Type    ServerMessageType
	LockTag string
}

// ClientMessage models a client-bound message.
type ClientMessage struct {
	Type    ClientMessageType
	LockTag string
}

// DecodeServerMessage decodes a slice of bytes into a ServerMessage pointer.
//
// There are a few possible errors:
//   - The length exceeds or is shorter than the possible bounds.
//   - The lock tag size does not match the communicated size.
//   - The server message type is not recognized.
//   - The lock tag is not valid UTF8.
func DecodeServerMessage(bytes []byte) (*ServerMessage, error) {
	log.Debug().
		Bytes("bytes", bytes).
		Msg("decoding server message")
	if len(bytes) < 3 || len(bytes) > 256 {
		return nil, ErrServerMessageDecode
	}
	log.Debug().Int("tag-size", int(bytes[1])).Send()
	if len(bytes[2:]) != int(bytes[1]) {
		return nil, ErrLockTagSize
	}
	messageType, err := decodeServerMessageType(bytes)
	log.Debug().Int("server-msg-type", int(messageType)).Send()
	if err != nil {
		return nil, err
	}
	lockTag, err := decodeLockTag(bytes)
	log.Debug().Str("lock-tag", lockTag).Send()
	if err != nil {
		return nil, err
	}

	return &ServerMessage{Type: messageType, LockTag: lockTag}, nil
}

// EncodeServerMessage converts a ServerMessage into a slice of bytes to be sent over a wire.
func EncodeServerMessage(serverMessage *ServerMessage) []byte {
	bytes := make([]byte, 2+len(serverMessage.LockTag))
	bytes[0] = byte(serverMessage.Type)
	bytes[1] = byte(len(serverMessage.LockTag))
	for i := 0; i < len(serverMessage.LockTag); i++ {
		bytes[i+2] = byte(serverMessage.LockTag[i])
	}
	return bytes
}

// DecodeClientMessage decodes a slice of bytes into a ClientMessage pointer.
//
// There are a few possible errors:
//   - The length exceeds or is shorter than the possible bounds.
//   - The lock tag size does not match the communicated size.
//   - The client message type is not recognized.
//   - The lock tag is not valid UTF8.
func DecodeClientMessage(bytes []byte) (*ClientMessage, error) {
	log.Debug().
		Bytes("bytes", bytes).
		Msg("decoding client message")
	if len(bytes) < 3 || len(bytes) > 256 {
		return nil, ErrClientMessageDecode
	}
	log.Debug().Int("tag-size", int(bytes[1])).Send()
	if len(bytes[2:]) != int(bytes[1]) {
		return nil, ErrLockTagSize
	}
	messageType, err := decodeClientMessageType(bytes)
	log.Debug().Int("client-msg-type", int(messageType)).Send()
	if err != nil {
		return nil, err
	}
	lockTag, err := decodeLockTag(bytes)
	log.Debug().Str("lock-tag", lockTag).Send()
	if err != nil {
		return nil, err
	}

	return &ClientMessage{Type: messageType, LockTag: lockTag}, nil
}

// EncodeServerMessage converts a ServerMessage into a slice of bytes to be sent over a wire.
func EncodeClientMessage(clientMessage *ClientMessage) []byte {
	log.Debug().Msg("encoding client message")
	bytes := make([]byte, 2+len(clientMessage.LockTag))
	log.Debug().Int("len", len(bytes)).Msg("made slice")
	bytes[0] = byte(Acquired)
	log.Debug().
		Bytes("bytes", bytes).
		Msg("added 'Acquired' message type")
	bytes[1] = byte(len(clientMessage.LockTag))
	log.Debug().
		Bytes("bytes", bytes).
		Msg("added lock tag size")
	log.Debug().
		Str("tag", clientMessage.LockTag).
		Msg("encoding lock tag")
	for i := 0; i < len(clientMessage.LockTag); i++ {
		bytes[i+2] = byte(clientMessage.LockTag[i])
	}
	log.Debug().
		Bytes("bytes", bytes).
		Msg("encoded client message")

	return bytes
}

// decodeserverMessageType attempts to extract the ServerMessageType from the given byte slice.
func decodeServerMessageType(bytes []byte) (ServerMessageType, error) {
	switch ServerMessageType(bytes[0]) {
	case Acquire:
		return Acquire, nil
	case Release:
		return Release, nil
	}
	return 0, ErrServerMessageType
}

// decodeserverMessageType attempts to extract the ClientMessageType from the given byte slice.
func decodeClientMessageType(bytes []byte) (ClientMessageType, error) {
	//nolint:gocritic
	switch ClientMessageType(bytes[0]) {
	case Acquired:
		return Acquired, nil
	}
	return 0, ErrClientMessageType
}

// decodeLockTag check whether the input byte slice contains a valid lock tag
// and if so returns is as a string.
func decodeLockTag(bytes []byte) (string, error) {
	lockTag := bytes[2:]
	if !utf8.Valid(lockTag) {
		return "", ErrLockTagEncoding
	}
	builder := strings.Builder{}
	builder.Write(lockTag)
	return builder.String(), nil
}
