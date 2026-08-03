package rudp

import (
	"bytes"
	"testing"
)

// pipe couples two sessions in-process: each session's SendFunc buffers datagrams
// for delivery to the other side on the next step.
type pipe struct {
	toServer [][]byte
	toClient [][]byte
}

func (p *pipe) clientSend(b []byte, _ bool) error {
	p.toServer = append(p.toServer, append([]byte(nil), b...))
	return nil
}

func (p *pipe) serverSend(b []byte, _ bool) error {
	p.toClient = append(p.toClient, append([]byte(nil), b...))
	return nil
}

func (p *pipe) step(server, client *Session) {
	deliver := p.toServer
	p.toServer = nil
	for _, m := range deliver {
		server.Feed(m)
	}
	deliver = p.toClient
	p.toClient = nil
	for _, m := range deliver {
		client.Feed(m)
	}
	_ = server.Pump()
	_ = client.Pump()
}

// TestCheckpoint3Handshake is CP3: a client and server session complete the
// SYN/SYN_ACK/ACK handshake and both reach Established.
func TestCheckpoint3Handshake(t *testing.T) {
	p := &pipe{}
	server := NewServerSession(p.serverSend, WithInitialSequence(1000))
	client := NewClientSession("player-one", p.clientSend, WithInitialSequence(2000))

	client.Connect()
	for i := 0; i < 50; i++ {
		if server.State() == StateEstablished && client.State() == StateEstablished {
			break
		}
		p.step(server, client)
	}
	if server.State() != StateEstablished {
		t.Fatalf("server state = %v, want Established", server.State())
	}
	if client.State() != StateEstablished {
		t.Fatalf("client state = %v, want Established", client.State())
	}
	t.Log("CP3 ok: both sessions reached Established")
}

func establish(t *testing.T, p *pipe, server, client *Session) {
	t.Helper()
	client.Connect()
	for i := 0; i < 50; i++ {
		if server.State() == StateEstablished && client.State() == StateEstablished {
			return
		}
		p.step(server, client)
	}
	t.Fatalf("failed to establish: server=%v client=%v", server.State(), client.State())
}

func TestDataRoundTrip(t *testing.T) {
	p := &pipe{}
	server := NewServerSession(p.serverSend, WithInitialSequence(1000))
	client := NewClientSession("player-one", p.clientSend, WithInitialSequence(2000))
	establish(t, p, server, client)

	// server -> client
	msg1 := []byte("hello from server")
	server.SendData(msg1)
	// client -> server
	msg2 := []byte("hello from client")
	client.SendData(msg2)

	var gotClient, gotServer []byte
	for i := 0; i < 50 && (gotClient == nil || gotServer == nil); i++ {
		p.step(server, client)
		if gotClient == nil {
			if d, ok := client.Receive(); ok {
				gotClient = d
			}
		}
		if gotServer == nil {
			if d, ok := server.Receive(); ok {
				gotServer = d
			}
		}
	}
	if !bytes.Equal(gotClient, msg1) {
		t.Errorf("client received %q, want %q", gotClient, msg1)
	}
	if !bytes.Equal(gotServer, msg2) {
		t.Errorf("server received %q, want %q", gotServer, msg2)
	}
}
