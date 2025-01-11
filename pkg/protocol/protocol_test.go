package protocol

import (
	"errors"
	"fmt"
	"testing"
)

func TestProtocol_decodeType(t *testing.T) {
	messages := [][]byte{
		{0}, {1},
	}

	for _, ty := range messages {
		_, err := decodeServerMessageType(ty)
		if err != nil {
			t.Error("Failed to decode ", ty)
		}
	}
}

func TestProtocol_decodeLockTag(t *testing.T) {
	messages := [][]byte{
		// Acquire
		{0, 9, 70, 70, 70, 70, 70, 70, 70, 70, 70},
		{0, 8, 70, 70, 70, 70, 70, 70, 70, 70},
		// Release
		{1, 9, 49, 49, 49, 49, 49, 49, 49, 49, 49},
		{1, 8, 49, 49, 49, 49, 49, 49, 49, 49},
	}
	expected := []string{
		"FFFFFFFFF",
		"FFFFFFFF",
		"111111111",
		"11111111",
	}

	for i, bytes := range messages {
		lockTag, err := decodeLockTag(bytes)
		if err != nil {
			t.Error("Failed to decode ", bytes, ": ", err)
		}
		if lockTag != expected[i] {
			t.Error(lockTag, "!=", expected[i])
		}
	}
}

func TestProtocol_messedUpMessages(t *testing.T) {
	acquireMessages := [][]byte{
		// Acquire
		{0, 9, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70},
		{0, 8, 70, 70, 70, 70, 70, 70, 70},
	}

	releaseMessages := [][]byte{
		// Release
		{1, 9},
		{1},
	}

	weirdMessages := [][]byte{
		// ????
		{100, 9, 70, 70, 70, 70, 70, 70, 70, 70, 70},
		{50, 8, 70, 70, 70, 70, 70, 70, 70, 70},
	}

	for _, message := range acquireMessages {
		_, err := DecodeServerMessage(message)
		if !errors.Is(err, ErrLockTagSize) {
			t.Error("Did not get error...")
		} else {
			t.Log("Got expected LockTagSizeError: ", err)
		}
	}

	for _, message := range releaseMessages {
		_, err := DecodeServerMessage(message)
		if !errors.Is(err, ErrServerMessageDecode) {
			t.Error("Did not get error...")
		} else {
			t.Log("Got expected ServerMessageDecodeError: ", err)
		}
	}

	for _, message := range weirdMessages {
		_, err := DecodeServerMessage(message)
		if !errors.Is(err, ErrServerMessageType) {
			t.Error("Did not get error...")
		} else {
			t.Log("Got expected ServerTypeError: ", err)
		}
	}
}

func TestProtocol_BadLockTagEncoding(t *testing.T) {
	messages := [][]byte{
		{0, 2, 0xc3, 0x28},
	}

	for _, message := range messages {
		_, err := DecodeServerMessage(message)
		if !errors.Is(err, ErrLockTagEncoding) {
			t.Error("Did not get error...")
		} else {
			t.Log("Got expected error: ", err)
		}
	}
}

// Verifies decoding of protocol messages works as intended.
//
// msg id  lock tag size  lock tag:
//
// 1 byte  1 byte         1 byte - 255 bytes
func TestProtocol_ServerMessage(t *testing.T) {
	// One byte array = 1 message
	messages := [][]byte{
		// Acquire
		{0, 9, 49, 49, 49, 49, 49, 49, 49, 49, 49},
		{0, 8, 49, 49, 49, 49, 49, 49, 49, 49},
		{0, 1, 70},
		{0, 254, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70,
			70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70,
			70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70,
			70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70,
			70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70,
			70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70},
		// Release
		{1, 9, 49, 49, 49, 49, 49, 49, 49, 49, 49},
		{1, 8, 49, 49, 49, 49, 49, 49, 49, 49},
		{1, 1, 70},
		{1, 254, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70,
			70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70,
			70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70,
			70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70,
			70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70,
			70, 70, 70, 70, 70, 70, 70, 70, 70, 70, 70},
	}
	expected := []*ServerMessage{
		{Type: Acquire, LockTag: string(messages[0][2:])},
		{Type: Acquire, LockTag: string(messages[1][2:])},
		{Type: Acquire, LockTag: string(messages[2][2:])},
		{Type: Acquire, LockTag: string(messages[3][2:])},
		{Type: Release, LockTag: string(messages[4][2:])},
		{Type: Release, LockTag: string(messages[5][2:])},
		{Type: Release, LockTag: string(messages[6][2:])},
		{Type: Release, LockTag: string(messages[7][2:])},
	}

	for i, bytes := range messages {
		t.Run(fmt.Sprintf("Run test iteration %d", i), func(t *testing.T) {
			t.Log(len(bytes))
			im, err := DecodeServerMessage(bytes)
			if err != nil {
				t.Error(err)
			}
			if im.Type != expected[i].Type {
				t.Error("Type did not match for iteration ", i)
			}
			if im.LockTag != expected[i].LockTag {
				t.Error("Lock tag did not match for iteration ", i)
				t.Errorf("Got: %s Expected: %s", im.LockTag, expected[i].LockTag)
			}
		})

	}
}

func TestProtocol_EncodeClientMessage(t *testing.T) {
	res := EncodeClientMessage(&ClientMessage{Type: Acquired, LockTag: "abc"})

	if len(res) != 5 {
		t.Error("Expected resulting byte array to have length 5")
	}

	cm, err := DecodeClientMessage(res)
	if err != nil {
		t.Error(err)
	}

	if cm.Type != Acquired {
		t.Error("Expected client message type to be Acquired")
	}

	if len(cm.LockTag) != 3 {
		t.Error("Expected locktag size to be 3")
	}
}

func Benchmark_Decoding(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = DecodeServerMessage([]byte{0, 9, 49, 49, 49, 49, 49, 49, 49, 49, 49})
	}
}
