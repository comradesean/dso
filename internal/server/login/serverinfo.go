package login

import (
	"fmt"
	"net/netip"

	"google.golang.org/protobuf/encoding/protowire"
)

// encodeServerInfoPS3 builds RequestQueryLoginServerInfoResponse the way the
// DARK SOULS 2 PS3 client (BLUS41045) actually parses it.
//
// The PC/DS3OS schema declares:
//
//	required int64  port      = 1;
//	required string server_ip = 2;
//
// The PS3 client's parser (vaddr 0x15E0370, reached via vtable+32 of the
// response class at 0x1C62DC8) accepts only fields 1, 2 and 3, and **gates every
// one of them on wiretype 0**:
//
//	clrlwi r0,r4,29 ; cmpwi r0,0 ; bne -> SkipField   (0x15E0498 for field 2)
//
// So there is no string field at all — the address is a plain 32-bit varint. The
// generated fast path even hardcodes `cmpwi r0,16` (0x15E0610) as the tag it
// expects after field 1, i.e. 0x10 = field 2 varint; protoc would have emitted
// 0x12 for a string.
//
// Sending it as a string is why this failed: tag 0x12 fails the wiretype gate,
// falls into SkipField (0x16568D8), and the member stays zero — so the client
// dials 0.0.0.0. Field 1 is unaffected, which is exactly the symptom we saw: the
// right port, a zero address. On the same host RPCS3 papers over it by
// redirecting 0.0.0.0 to 127.0.0.1; from another machine it fails outright.
//
// Byte order: the value reaches sin_addr with no conversion (MakeSockAddrIn at
// 0x17C05E0 does `stw r4,4(r3)` with no htonl, and the binary contains no
// byte-reverse instructions at all), so it must be (a<<24)|(b<<16)|(c<<8)|d.
// 192.168.1.100 is 0xC0A80164. Same convention as the 56-byte GameServerInfo.
//
// Field 3 exists in the schema but the login task never reads it, so it is
// omitted. Fields are emitted in ascending order so the client takes its
// optimistic fast path rather than the fallback parser.
func encodeServerInfoPS3(authIP string, authPort uint16) ([]byte, error) {
	addr, err := netip.ParseAddr(authIP)
	if err != nil {
		return nil, fmt.Errorf("login: auth server ip %q is not an IP address: %w", authIP, err)
	}
	addr = addr.Unmap()
	if !addr.Is4() {
		return nil, fmt.Errorf("login: auth server ip %q is not IPv4; the client's field is 32-bit", authIP)
	}
	v4 := addr.As4()
	ipValue := uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])

	var buf []byte
	buf = protowire.AppendTag(buf, 1, protowire.VarintType)
	buf = protowire.AppendVarint(buf, uint64(authPort))
	buf = protowire.AppendTag(buf, 2, protowire.VarintType)
	buf = protowire.AppendVarint(buf, uint64(ipValue))
	return buf, nil
}

// decodeServerInfoPS3 parses the PS3 form back, for tests and the client
// emulator. Returns the auth address and port.
func decodeServerInfoPS3(b []byte) (netip.Addr, uint16, error) {
	var (
		port uint16
		ip   uint32
		seen int
	)
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return netip.Addr{}, 0, fmt.Errorf("login: bad tag in server info")
		}
		b = b[n:]
		if typ != protowire.VarintType {
			return netip.Addr{}, 0, fmt.Errorf("login: field %d has wiretype %d, want varint", num, typ)
		}
		v, n := protowire.ConsumeVarint(b)
		if n < 0 {
			return netip.Addr{}, 0, fmt.Errorf("login: bad varint for field %d", num)
		}
		b = b[n:]
		switch num {
		case 1:
			port = uint16(v)
			seen |= 1
		case 2:
			ip = uint32(v)
			seen |= 2
		}
	}
	if seen != 3 {
		return netip.Addr{}, 0, fmt.Errorf("login: server info missing port or address (seen mask %d)", seen)
	}
	return netip.AddrFrom4([4]byte{
		byte(ip >> 24), byte(ip >> 16), byte(ip >> 8), byte(ip),
	}), port, nil
}
