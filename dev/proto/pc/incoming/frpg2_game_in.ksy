meta:
  id: frpg2_game_in
  title: Frpg2 game service - client to server (reliable UDP)
  file-extension: bin
  endian: be
  ks-version: 0.9

doc: |
  One UDP datagram sent by a Dark Souls 2/3 client to the GAME service (UDP port 50000 or 50001).

  *** CONFIRMED AGAINST LIVE PC TRAFFIC (2026-08-07). ***
  Nine sessions against FromSoftware's own servers were decrypted with keys pulled
  from the running client (see tasks/pc-capture-decryption.md), giving ~4,700
  messages across 28 opcodes with zero decryption failures. Every layer below was
  read off real datagrams. The PS3 game service remains unverified; the layers are
  believed shared but only the PC side has been observed.

  PORT: the live game service runs on **UDP 50000 or 50001**, varying per session --
  NOT 50010. Do not hardcode it.

  There are four nested layers, and they must be peeled in order:

    1. datagram envelope   token, IV, tag, packet-type byte  (this file, top level)
    2. reliable-UDP packet 7-byte header, SYN/ACK/DAT/...    (`rudp_packet`)
    3. fragment            12-byte header, reassembly        (`fragment`)
    4. message             12-byte header + protobuf         (`game_message`)

  The client->server datagram carries TWO things the server->client one does not: the
  8-byte auth token in the clear, and a packet-type byte. That asymmetry is the reason
  this spec is separate from frpg2_game_out.ksy.

seq:
  - id: auth_token
    size: 8
    doc: |
      The token issued by the auth service in `game_server_info`, sent IN THE CLEAR.
      It is what lets a connectionless listener find the session's CWC key before it
      can decrypt anything - the server looks this up, then decrypts. Present on every
      client->server datagram, not just the first.
  - id: iv
    size: 11
    doc: Random per datagram. Also part of the AAD (see below).
  - id: tag
    size: 16
    doc: CWC authentication tag.
  - id: packet_type
    type: u1
    enum: packet_type
    doc: |
      1 when the plaintext begins with the 35-byte connection prefix (i.e. the SYN
      datagram), 0 otherwise. Authenticated but not encrypted.
  - id: ciphertext
    size-eos: true
    doc: |
      AES-CWC-128. The AAD for this direction is `iv || auth_token || packet_type` -
      note it is NOT just the IV, unlike the TCP cipher and unlike the server->client
      UDP direction. Decrypts to `connection_prefix` (if packet_type == 1) followed by
      a `rudp_packet`, or just a `rudp_packet`.

types:
  connection_prefix:
    doc: |
      35-byte block prepended to the plaintext of the SYN datagram only. Carries the
      player's platform account id twice, 17 bytes each with a separating zero byte.
      Sent only by the client - the server never emits one.
    seq:
      - id: id_first
        size: 17
        type: strz
        encoding: ASCII
      - id: separator
        type: u1
        doc: Zero.
      - id: id_second
        size: 17
        type: strz
        encoding: ASCII
        doc: The same id repeated. Purpose of the duplication is unknown.

  rudp_packet:
    doc: |
      The reliable-UDP layer: ordering, retransmission and heartbeats over UDP.

      Sequence numbers are 12-bit and wrap at 4096, packed awkwardly across three
      bytes (see below). Only DAT, DAT_ACK and FIN_ACK consume a sequence number and
      go through the retransmit queue; everything else is sent immediately.
    seq:
      - id: magic0
        contents: [0xF5]
      - id: magic1
        contents: [0x02]
      - id: local_low
        type: u1
        doc: Low 8 bits of the sender's 12-bit sequence number.
      - id: seq_high
        type: u1
        doc: |
          Packed nibbles: the HIGH nibble holds bits 8-11 of `local`, the LOW nibble
          holds bits 8-11 of `remote`. So:
            local  = local_low  | ((seq_high >> 4)  << 8)
            remote = remote_low | ((seq_high & 0x0F) << 8)
      - id: remote_low
        type: u1
        doc: Low 8 bits of the acknowledged remote sequence number.
      - id: opcode
        type: u1
        enum: opcode
      - id: trailer
        contents: [0xFF]
        doc: Constant 0xFF. Purpose unknown; the client rejects other values.
      - id: payload
        size-eos: true
        doc: |
          For SYN this is the fixed 8 bytes 12 10 20 20 00 00 A0 00.
          For DAT / DAT_ACK it is a `fragment`.
          For ACK / HBT / FIN / RST it is usually empty.

  fragment:
    doc: |
      Splits a message across several DAT packets. A message under the fragment limit
      still gets exactly one fragment, so this layer is always present on data packets.
    seq:
      - id: packet_counter
        type: u2le
        doc: LITTLE-ENDIAN, unlike the rest of this header.
      - id: compress_flag
        type: u1
        doc: |
          Non-zero if the reassembled payload is zlib-compressed. When set, fragment
          index 0 carries an extra u4be decompressed-length immediately after this
          12-byte header, before the body. Compression kicks in above ~512 bytes.
      - id: unknown_1
        size: 3
        doc: Zero in every observed fragment.
      - id: total_payload_length
        type: u2
        doc: Total length of the reassembled payload across all fragments.
      - id: unknown_2
        type: u1
        doc: Zero.
      - id: fragment_index
        type: u1
        doc: 0-based index of this fragment.
      - id: fragment_length
        type: u2
        doc: Length of this fragment's body.
      - id: body
        size: fragment_length
        doc: |
          Concatenate bodies in index order to reassemble. The result (after zlib
          inflate if compress_flag was set) is a `game_message`.

  game_message:
    doc: |
      The message layer, same shape as the TCP one but with a different response-header
      byte order.
    seq:
      - id: header_size
        type: u4
        doc: |
          12 on every message observed. NOTE: on server->client RESPONSES the protobuf
          does not begin at offset 12 -- an extra 16 bytes sit between msg_index and the
          protobuf, so the body starts at 28. Requests start at 12.

          IDENTIFIED 2026-08-07: those 16 bytes are the response header, `{0, 1, 0, 0}`
          written LITTLE-endian (the TCP response header holds the same values
          big-endian; they look alike in a hexdump only because the values are small).

          DERIVE the offset from the header, never probe for it. cmd/corpus used to
          trial-parse starting at 8, and offset 8 is msg_index -- four bytes of index
          parse as valid protobuf often enough that 6,515 of 15,573 files, 42% of the
          corpus, were written with the index prepended to their payload. Verified after
          the fact: in every one of those files the first four payload bytes read as a
          little-endian u32 equal the file's own index. The rule is simply
          `12, or 28 when msg_type == 0`.
      - id: msg_type
        type: u4
        doc: |
          The game-service opcode. CONFIRMED on PC: all 28 opcodes observed live land
          exactly where docs/protocol-map.md places them, so DS2 SOTFS on PC shares the
          numbering mapped from the PS3 binary.

          RESPONSES CARRY msg_type 0. They are matched to their request by msg_index,
          not by an echoed opcode -- 7,739 of the 15,573-message corpus are these, and
          every one was successfully attributed to a request that way.

          The protobuf body may contain GROUPS (wire types 3/4), the deprecated
          construct. DS2 still uses them: RequestNotifyKillEnemy and the
          RequestGetRightMatchingArea response are both groups, and a reader that
          rejects wire type 3 stops mid-message on live traffic.

          Special case CONFIRMED LIVE: 0x0320 client->server is RequestSendMessageToPlayers,
          but 0x0320 server->client with msg_index 0xFFFFFFFF is a PUSH, and the real push
          id is protobuf field 1. Direction matters. Push ids seen on the wire:

            908  (0x038C) PlayerInfoUploadConfigPushMessage
            938  (0x03AA) PushRequestEvaluateBloodMessage
            972  (0x03CC) Bell Keeper visitor push
            1007 (0x03EF) PushRequestNotifyRingBell

          Filing captured messages by msg_type alone buries every push type in one
          bucket -- split on this field instead.
      - id: msg_index
        type: u4le
        doc: LITTLE-ENDIAN.
      - id: protobuf_body
        size-eos: true
        doc: |
          Protobuf; see docs/protocol-map.md for the per-opcode message. The first
          message a client sends is RequestWaitForUserLogin (0x0386), whose reply
          carries the assigned player_id.

enums:
  packet_type:
    0: normal
    1: connection_prefix

  opcode:
    0x02: syn
    0x03: rack
    0x04: dat
    0x05: hbt
    0x06: fin
    0x07: rst
    0x08: dat_frag
    0x31: ack
    0x32: syn_ack
    0x34: dat_ack
    0x36: fin_ack
    0x38: dat_frag_ack
