package packet

import (
	"bytes"
	"net"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	writer := NewStream(c1)
	reader := NewStream(c2)

	payloads := [][]byte{
		{},
		{0x01},
		bytes.Repeat([]byte{0xAB}, 500),
	}

	go func() {
		for _, p := range payloads {
			if err := writer.WritePacket(p); err != nil {
				t.Errorf("write: %v", err)
				return
			}
		}
	}()

	for i, want := range payloads {
		got, err := reader.ReadPacket()
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("packet %d: got %x want %x", i, got, want)
		}
	}
}

func TestWireFormatGolden(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		s := NewStream(c1)
		_ = s.WritePacket([]byte{0xDE, 0xAD, 0xBE, 0xEF})
	}()

	buf := make([]byte, 2+headerSize+4)
	if _, err := readFull(c2, buf); err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x00, 0x10, // outer length = 16 (12 header + 4 payload)
		0x00, 0x01, // send_counter = 1
		0x00, 0x00, // unknown_1, unknown_2
		0x00, 0x00, 0x00, 0x04, // payload_length = 4 (BE)
		0x00, 0x00, // unknown_3
		0x00, 0x04, // payload_length_short = 4 (BE)
		0xDE, 0xAD, 0xBE, 0xEF,
	}
	if !bytes.Equal(buf, want) {
		t.Fatalf("wire bytes:\n got %x\nwant %x", buf, want)
	}
}

func readFull(c net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := c.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
