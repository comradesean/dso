package auth

import (
	"encoding/binary"
	"fmt"

	"github.com/sstreight/dso/internal/server/authtoken"
)

// gameServerInfoSize is the fixed size of the Frpg2GameServerInfo struct.
const gameServerInfoSize = 184

// tuning are the 11 trailing uint32 constants the client expects (buffer sizes
// of some description), emitted big-endian.
var tuning = [11]uint32{
	0x00008000, 0x00008000, 0x0000A000, 0x0000A000, 0x00000080,
	0x00008000, 0x0000A000, 0x000493E0, 0x000061A8, 0x0000000C, 0x00000000,
}

// encodeGameServerInfo builds the raw 184-byte Frpg2GameServerInfo response.
// Layout (packed):
//
//	[0]   auth_token       u64  (8 raw bytes, not byte-swapped)
//	[8]   game_server_ip   [16] (NUL-terminated ASCII, not byte-swapped)
//	[24]  stack_data       [112] (zeroed)
//	[136] game_port        u16  big-endian
//	[138] padding          u16  = 0
//	[140] tuning[11]        u32  big-endian each
func encodeGameServerInfo(token authtoken.Token, gameServerIP string, gamePort uint16) ([]byte, error) {
	if len(gameServerIP)+1 > 16 {
		return nil, fmt.Errorf("auth: game server ip %q too long for 16-byte field", gameServerIP)
	}
	buf := make([]byte, gameServerInfoSize)
	copy(buf[0:8], token[:])
	copy(buf[8:24], gameServerIP) // remainder stays NUL
	// stack_data [24:136] stays zero
	binary.BigEndian.PutUint16(buf[136:138], gamePort)
	// padding [138:140] stays zero
	for i, v := range tuning {
		off := 140 + i*4
		binary.BigEndian.PutUint32(buf[off:off+4], v)
	}
	return buf, nil
}
