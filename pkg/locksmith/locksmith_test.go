package locksmith

import (
	"context"
	"testing"
)

/*
How to decode TCP packages received by the Locksmith server:

In a loop:

rest = []byte
rest_index = 0
buffer = []byte

// Read from connection
while True:

	pos = 0
	n = read(buffer)

	// Handle rest item first, rest_index being set means there was an overlap between packets we need to handle.
	if rest_index > 0:
		tag_size = int(rest[1])

		// determine remaining lock tag characters to get from the buffer
		// rest_index = 7
		// 0 1 2 3 4 5 6
		// 0 7 a a a a a
		remaining_chars = tag_size - (rest_index - 2)
		// remaining_chars will be = 2

		rest_index = 0

	while pos != n:

		tag_size = int(buffer[pos + 1])

		if (tag_size + 2) <= (n - pos):
			handle_message(buffer[pos:pos+tag_size+2])

			pos = pos + tag_size + 2
		elif tag_size + 2 > n:
			n_copied = copy(rest, buffer[pos:n])
			rest_index = n_copied
			pos = pos + n_copied
*/
func TestBytesBuffer(t *testing.T) {
	buffer := make([]byte, 257)
	read := []byte{1, 12, 3, 4, 5, 6, 7}

	t.Log(int(read[1]))

	buffer[4] = 12
	t.Log(buffer)
	t.Log(len(buffer))

	copy(buffer, read[1:])
	t.Log(buffer)
	t.Log(len(buffer))

}

func TestServer_Stop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	locksmith := New(&LocksmithOptions{Port: 30001})

	go func() {
		cancel()
	}()

	err := locksmith.Start(ctx)
	if err != nil {
		t.Error("Error from Locksmith.Start: ", err)
	}

	t.Log("Locksmith stopped")
}
