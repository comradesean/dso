meta:
  id: frpg2_game_out
  title: Frpg2 game service - server to client (reliable UDP)
  file-extension: bin
  endian: be
  ks-version: 0.9

doc: |
  One UDP datagram sent by the GAME service back to a Dark Souls 2/3 client (port 50010).

  *** NOT CONFIRMED AGAINST A PS3 CLIENT. *** Same caveat as frpg2_game_in.ksy: no
  game-service byte has ever been observed from a real PS3 client. Derived from our Go
  implementation and the PC-targeting DS3OS reference. Hypothesis, not ground truth.

  The server->client datagram is MUCH simpler than the inbound one: no auth token, no
  packet-type byte, and the AAD is just the IV. The server already knows which session
  a datagram belongs to because it is replying to a known peer address, so none of the
  demux machinery is needed in this direction.

    inbound  : token(8) || iv(11) || tag(16) || ptype(1) || ciphertext   AAD = iv||token||ptype
    outbound :             iv(11) || tag(16)             || ciphertext   AAD = iv

  Nested layers below the envelope are the same as inbound: reliable-UDP packet ->
  fragment -> message. The server never sends a connection prefix.

seq:
  - id: iv
    size: 11
    doc: Random per datagram. Serves as the AAD by itself in this direction.
  - id: tag
    size: 16
    doc: CWC authentication tag over (iv as AAD, ciphertext).
  - id: ciphertext
    size-eos: true
    doc: |
      AES-CWC-128, same cipher as the TCP stream. Decrypts directly to a `rudp_packet` -
      there is never a connection prefix to strip here.

types:
  rudp_packet:
    doc: |
      Identical layout to the inbound direction. The opcodes the server actually emits
      are SYN_ACK and ACK during connection setup, DAT / DAT_ACK for data, HBT for
      heartbeats, and FIN_ACK on teardown.

      Worth knowing when reading a capture: the session does NOT reach Established on
      the first SYN_ACK. Both peers move to a SynReceived state first and the following
      ACK exchange completes it - a decoder (or a test) that expects Established
      immediately after SYN_ACK is wrong.
    seq:
      - id: magic0
        contents: [0xF5]
      - id: magic1
        contents: [0x02]
      - id: local_low
        type: u1
        doc: Low 8 bits of the server's 12-bit sequence number.
      - id: seq_high
        type: u1
        doc: |
          High nibble = bits 8-11 of `local`; low nibble = bits 8-11 of `remote`.
            local  = local_low  | ((seq_high >> 4)  << 8)
            remote = remote_low | ((seq_high & 0x0F) << 8)
      - id: remote_low
        type: u1
        doc: Low 8 bits of the client sequence number being acknowledged.
      - id: opcode
        type: u1
        enum: opcode
      - id: trailer
        contents: [0xFF]
      - id: payload
        size-eos: true
        doc: |
          For SYN_ACK the fixed 8 bytes 12 10 20 20 00 01 00 00 (note this differs from
          the client's SYN payload only in bytes 5-6).
          For DAT / DAT_ACK a `fragment`. For ACK / HBT usually empty.

  fragment:
    doc: Identical to the inbound fragment layer.
    seq:
      - id: packet_counter
        type: u2le
        doc: LITTLE-ENDIAN.
      - id: compress_flag
        type: u1
        doc: |
          Non-zero if the reassembled payload is zlib-compressed; fragment 0 then
          carries an extra u4be decompressed length before its body.
      - id: unknown_1
        size: 3
      - id: total_payload_length
        type: u2
      - id: unknown_2
        type: u1
      - id: fragment_index
        type: u1
      - id: fragment_length
        type: u2
      - id: body
        size: fragment_length

  game_message:
    doc: |
      Server->client messages come in two kinds and the distinction is only visible
      here, not at the transport layer:

        REPLY - msg_type 0, msg_index copied from the request, and a 16-byte response
                header between the header and the protobuf.
        PUSH  - msg_type 0x0320 and msg_index 0xFFFFFFFF, unsolicited. The actual push
                identity lives in the FIRST PROTOBUF FIELD (PushMessageId), not in
                msg_type. So every push looks identical at this layer.

      This is why 0x0320 appears to collide: client->server it is
      RequestSendMessageToPlayers; server->client it means "this is a push".
    seq:
      - id: header_size
        type: u4
        doc: Always 12.
      - id: msg_type
        type: u4
        doc: 0 for a reply, 0x0320 for a push.
      - id: msg_index
        type: u4le
        doc: |
          LITTLE-ENDIAN. Copied from the request for a reply; 0xFFFFFFFF for a push.
      - id: response_header
        type: response_header
        if: msg_type == 0
        doc: Present on replies only, never on pushes.
      - id: protobuf_body
        size-eos: true
        doc: |
          Protobuf. For a push, field 1 is the PushMessageId identifying which push
          this is. See docs/protocol-map.md.

  response_header:
    doc: |
      {0, 1, 0, 0} written LITTLE-ENDIAN here - the TCP response header holds the same
      values BIG-endian. The two look identical in a hexdump only because the values
      are small; do not share a codec between them.
    seq:
      - id: unknown_0
        type: u4le
      - id: unknown_1
        type: u4le
        doc: Always 1.
      - id: unknown_2
        type: u4le
      - id: unknown_3
        type: u4le

enums:
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
