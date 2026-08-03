package rudp

import (
	"bytes"
	"testing"
)

// TestMessageConnRequestReply drives a request and a reply through the full
// message + fragment + reliable-UDP stack between two in-process sessions.
func TestMessageConnRequestReply(t *testing.T) {
	p := &pipe{}
	server := NewServerSession(p.serverSend, WithInitialSequence(1000))
	client := NewClientSession("player-one", p.clientSend, WithInitialSequence(2000))
	establish(t, p, server, client)

	sc := NewMessageConn(server)
	cc := NewMessageConn(client)

	const opRequestWaitForUserLogin = 0x0386
	reqBody := []byte("wait-for-user-login-request")
	cc.SendRequest(opRequestWaitForUserLogin, reqBody)

	// Deliver the request to the server.
	var got Message
	var gotOK bool
	for i := 0; i < 50 && !gotOK; i++ {
		p.step(server, client)
		m, ok, err := sc.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			got, gotOK = m, true
		}
	}
	if !gotOK {
		t.Fatal("server did not receive the request")
	}
	if got.Type != opRequestWaitForUserLogin {
		t.Errorf("request type = %#x, want %#x", got.Type, opRequestWaitForUserLogin)
	}
	if !bytes.Equal(got.Payload, reqBody) {
		t.Errorf("request payload = %q, want %q", got.Payload, reqBody)
	}

	// Server replies.
	replyBody := []byte("player_id=1")
	sc.SendReply(got, replyBody)

	var reply Message
	var replyOK bool
	for i := 0; i < 50 && !replyOK; i++ {
		p.step(server, client)
		m, ok, err := cc.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			reply, replyOK = m, true
		}
	}
	if !replyOK {
		t.Fatal("client did not receive the reply")
	}
	if !reply.IsReply {
		t.Errorf("reply IsReply = false, want true")
	}
	if reply.Index != got.Index {
		t.Errorf("reply index = %d, want %d (copied from request)", reply.Index, got.Index)
	}
	if !bytes.Equal(reply.Payload, replyBody) {
		t.Errorf("reply payload = %q, want %q", reply.Payload, replyBody)
	}
}

// TestFragmentReassembly checks that a payload larger than one fragment is split
// and reassembled correctly.
func TestFragmentReassembly(t *testing.T) {
	p := &pipe{}
	server := NewServerSession(p.serverSend, WithInitialSequence(1000))
	client := NewClientSession("player-one", p.clientSend, WithInitialSequence(2000))
	establish(t, p, server, client)

	sc := NewMessageConn(server)
	cc := NewMessageConn(client)

	big := make([]byte, 2500) // spans 3 fragments (900 each)
	for i := range big {
		big[i] = byte(i)
	}
	cc.SendRequest(0x0400, big)

	for i := 0; i < 100; i++ {
		p.step(server, client)
		m, ok, err := sc.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			if !bytes.Equal(m.Payload, big) {
				t.Fatalf("reassembled payload mismatch (len %d)", len(m.Payload))
			}
			return
		}
	}
	t.Fatal("multi-fragment message was not received")
}
