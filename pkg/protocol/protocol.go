// Package protocol implements decoding and encoding of client/server messages of the Locksmith protocol.
package protocol

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/trebent/zerologr"
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
	zerologr.V(50).Info("Decoding server message", "bytes", bytes)
	if len(bytes) < 3 || len(bytes) > 256 {
		return nil, ErrServerMessageDecode
	}
	zerologr.V(50).Info("", "tag-size", int(bytes[1]))
	if len(bytes[2:]) != int(bytes[1]) {
		return nil, ErrLockTagSize
	}
	messageType, err := decodeServerMessageType(bytes)
	zerologr.V(50).Info("", "server-msg-type", int(messageType))
	if err != nil {
		return nil, err
	}
	lockTag, err := decodeLockTag(bytes)
	zerologr.V(50).Info("", "lock-tag", lockTag)
	if err != nil {
		return nil, err
	}

	return &ServerMessage{Type: messageType, LockTag: lockTag}, nil
}

// EncodeServerMessage converts a ServerMessage into a slice of bytes to be sent over a wire.
func EncodeServerMessage(serverMessage *ServerMessage) []byte {
	const magic = 2
	bytes := make([]byte, magic+len(serverMessage.LockTag))
	bytes[0] = byte(serverMessage.Type)
	//nolint:gosec // this is fine
	bytes[1] = byte(len(serverMessage.LockTag))
	for i := range len(serverMessage.LockTag) {
		bytes[i+2] = serverMessage.LockTag[i]
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
	zerologr.V(50).Info("Decoding client message", "bytes", bytes)
	if len(bytes) < 3 || len(bytes) > 256 {
		return nil, ErrClientMessageDecode
	}
	zerologr.V(50).Info("", "tag-size", int(bytes[1]))
	if len(bytes[2:]) != int(bytes[1]) {
		return nil, ErrLockTagSize
	}
	messageType, err := decodeClientMessageType(bytes)
	zerologr.V(50).Info("", "client-msg-type", int(messageType))
	if err != nil {
		return nil, err
	}
	lockTag, err := decodeLockTag(bytes)
	zerologr.V(50).Info("", "lock-tag", lockTag)
	if err != nil {
		return nil, err
	}

	return &ClientMessage{Type: messageType, LockTag: lockTag}, nil
}

// EncodeClientMessage converts a ServerMessage into a slice of bytes to be sent over a wire.
func EncodeClientMessage(clientMessage *ClientMessage) []byte {
	const magic = 2

	zerologr.V(50).Info("Encoding client message")
	bytes := make([]byte, magic+len(clientMessage.LockTag))
	zerologr.V(50).Info("Made slice", "len", len(bytes))
	bytes[0] = byte(Acquired)
	zerologr.V(50).Info("Added 'Acquired' message type", "bytes", bytes)
	//nolint:gosec // this is fine
	bytes[1] = byte(len(clientMessage.LockTag))
	zerologr.V(50).Info("Added lock tag size", "bytes", bytes)
	zerologr.V(50).Info("Encoding lock tag", "tag", clientMessage.LockTag)
	for i := range len(clientMessage.LockTag) {
		bytes[i+2] = clientMessage.LockTag[i]
	}
	zerologr.V(50).Info("Encoded client message", "bytes", bytes)

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
//
//nolint:unparam // why
func decodeClientMessageType(bytes []byte) (ClientMessageType, error) {
	//nolint:gocritic // why
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
