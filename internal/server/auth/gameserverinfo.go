package auth

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"github.com/sstreight/dso/internal/server/authtoken"
)

// gameServerInfoSize is the size of the Frpg2GameServerInfo response the DS2 PS3
// client expects.
//
// The client enforces this as a HARD EQUALITY CHECK. Recovered from BLUS41045 at
// vaddr 0x167091c:
//
//	lwz   r0,120(r1)        ; payload.end
//	subf  r0,r11,r0         ; len = end - begin
//	cmpwi cr7,r0,56         ; must be exactly 56
//	beq   cr7,0x1670a18     ; only then copy the struct
//
// Any other length falls through to a plain return: no error, no log, no state
// change — but the struct is never copied, so the game server address, port and
// every transport parameter stay zero. The client then "connects" to 0.0.0.0:0
// with zero-sized socket buffers and silently never sends a datagram. That
// silent-skip is exactly the symptom we chased: auth completing, the client
// binding a UDP socket, and then never using it.
const gameServerInfoSize = 56

// tuning are the ten trailing uint32 transport parameters, emitted big-endian.
//
// These are the PS3 client's own defaults, recovered from the default-constructed
// transport config at 0x165f894-0x165f92c. They differ from the PC/SOTFS values
// that the ds3os reference sends: the two buffer-size classes are halved on PS3
// (0x8000 -> 0x4000 and 0xA000 -> 0x5000). Sending the client's own defaults
// avoids having to understand what each one configures.
//
// The first two are load-bearing and are applied verbatim to the game socket:
// index 0 becomes SO_SNDBUF and index 1 becomes SO_RCVBUF (param-id jump table at
// 0x17ca2f0, applied at 0x17a3dc4 / 0x17a3e24). Every one of the ten is pushed
// through a SetParam call whose failure aborts connection setup outright, so a
// nonsense value here stops the client dead rather than degrading it.
//
// Note there are TEN, not eleven: the PC struct's trailing zero uint32 does not
// exist on PS3.
var tuning = [10]uint32{
	0x00004000, // -> SO_SNDBUF
	0x00004000, // -> SO_RCVBUF
	0x00005000,
	0x00005000,
	0x00000080,
	0x00004000,
	0x00005000,
	0x000493E0,
	0x000061A8,
	0x0000000C,
}

// encodeGameServerInfo builds the raw 56-byte Frpg2GameServerInfo response that
// tells the client where the UDP game service is.
//
// Layout (all multi-byte fields big-endian), recovered from the field copy at
// 0x1670a18-0x1670a8c:
//
//	[0]  auth_token      8 opaque bytes, copied verbatim, not byte-swapped
//	[8]  game_server_ip  u32, RAW BINARY a.b.c.d — NOT an ASCII string
//	[12] game_port       u16
//	[14] padding         u16, zero
//	[16] tuning[10]      u32 each
//
// The IP being binary rather than ASCII is the biggest structural difference from
// the PC layout. It goes straight into sin_addr with no conversion: MakeSockAddrIn
// at 0x17c05e0 does `stw r4,4(r3)` / `sth r5,2(r3)` with no htonl/htons, and
// inet_addr is never called anywhere on this path. So the four bytes on the wire
// are the address octets in order.
func encodeGameServerInfo(token authtoken.Token, gameServerIP string, gamePort uint16) ([]byte, error) {
	addr, err := netip.ParseAddr(gameServerIP)
	if err != nil {
		return nil, fmt.Errorf("auth: game server ip %q is not an IP address: %w", gameServerIP, err)
	}
	// The field is four bytes; an IPv6 address cannot be expressed here.
	addr = addr.Unmap()
	if !addr.Is4() {
		return nil, fmt.Errorf("auth: game server ip %q is not IPv4; the client's field is 4 bytes", gameServerIP)
	}
	v4 := addr.As4()

	buf := make([]byte, gameServerInfoSize)
	copy(buf[0:8], token[:])
	copy(buf[8:12], v4[:])
	binary.BigEndian.PutUint16(buf[12:14], gamePort)
	// padding [14:16] stays zero
	for i, v := range tuning {
		off := 16 + i*4
		binary.BigEndian.PutUint32(buf[off:off+4], v)
	}
	return buf, nil
}
