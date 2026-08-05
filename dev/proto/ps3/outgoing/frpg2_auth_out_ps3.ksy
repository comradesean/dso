meta:
  id: frpg2_auth_out_ps3
  title: Frpg2 auth service - server to client, PS3 game_server_info (decomp-derived)
  file-extension: bin
  endian: be
  ks-version: 0.9

doc: |
  The stage-4 auth reply payload as the DARK SOULS 2 PS3 client (BLUS41045) actually
  parses it.

  DERIVED BY DECOMPILATION of the retail EBOOT, not from the PC reference. Where this
  disagrees with dev/proto/pc/outgoing/frpg2_auth_out.ksy, this file wins for PS3.

  The framing around this payload (packet header, message header, response header,
  CWC envelope) is unchanged from the PC spec - only this struct differs. Parse the
  envelope with the PC auth spec, then parse the decrypted stage-4 body with this.

  WHY THIS MATTERS: the client checks the payload length with a HARD EQUALITY against
  56 and, on any mismatch, skips the entire struct copy without logging anything or
  changing state. The address, port and all ten transport parameters stay zero, so it
  binds 0.0.0.0:0 with zero-sized socket buffers and never sends a datagram. Sending
  the PC 184-byte struct produces a client that authenticates perfectly and then
  appears to "choose" not to play - with no error anywhere.

    0x167091c:  lwz   r0,120(r1)     ; payload.end
                subf  r0,r11,r0      ; len = end - begin
                cmpwi cr7,r0,56      ; must be exactly 56
                beq   cr7,0x1670a18  ; only then copy

seq:
  - id: auth_token
    size: 8
    doc: |
      Eight opaque bytes, copied verbatim with no byte swapping. The client prefixes
      this to every client->server game datagram in the clear so the server can find
      the session's CWC key before decrypting. Stored at conn+896 (0x16657a4).
  - id: game_server_ip
    type: u4
    doc: |
      RAW BINARY IPv4 address, NOT an ASCII string - the single biggest structural
      difference from the PC layout. 192.168.1.100 is the four bytes C0 A8 01 64.

      It reaches sin_addr with no conversion at all: MakeSockAddrIn (0x17c05e0) does
      `stw r4,4(r3)` with no htonl, and inet_addr is never called on this path. So the
      wire bytes are the address octets in order.
  - id: game_port
    type: u2
    doc: |
      UDP game service port, big-endian, straight into sin_port via `sth r5,2(r3)`
      with no htons (50010).
  - id: padding
    type: u2
    doc: |
      Copied to the auth state object at +174 but no reader was found for it. Send zero.
  - id: so_sndbuf
    type: u4
    doc: |
      Applied VERBATIM as SO_SNDBUF on the game socket (param id 12 -> optname 0x1001
      via the jump table at 0x17ca2f0; applied at 0x17a3e24).
      PS3 client default: 0x4000. The PC reference sends 0x8000.
      A zero here gives the client a zero-sized send buffer.
  - id: so_rcvbuf
    type: u4
    doc: |
      Applied VERBATIM as SO_RCVBUF (param id 9 -> optname 0x1002; applied at
      0x17a3dc4). PS3 client default: 0x4000. PC reference sends 0x8000.
  - id: transport_params
    type: u4
    repeat: expr
    repeat-expr: 8
    doc: |
      The remaining eight transport parameters, in wire order at offsets 24..52.
      PS3 client defaults:
        0x5000, 0x5000, 0x0080, 0x4000, 0x5000, 0x493E0, 0x61A8, 0x000C

      Compared with PC, the two buffer-size classes are halved (0x8000 -> 0x4000,
      0xA000 -> 0x5000); the last three match. Offsets 36..52 map to internal
      transport param ids 35, 34, 41, 40 and 39 respectively (0x17a3ebc, 0x17a3e6c,
      0x17a3f0c, 0x17a3f5c, 0x17a3fac) which are handled by a transport virtual rather
      than sys_net, so their exact meaning is not established - sending the client's
      own defaults sidesteps the question.

      EVERY one of the ten values is pushed through a SetParam call guarded by
      `beq cr7,0x17a421c`; if any single one is rejected, CreateConnection returns NULL
      and no UDP traffic ever occurs.

      NOTE there are TEN total (2 named above + 8 here). The PC struct has ELEVEN; its
      trailing zero u4 does not exist on PS3.
