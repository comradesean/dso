meta:
  id: frpg2_game_out_ps3
  title: Frpg2 game service - server to client, PS3 (reliable UDP)
  file-extension: bin
  endian: be
  ks-version: 0.9

doc: |
  One UDP datagram sent by the game service back to a DARK SOULS 2 PS3 client
  (BLUS41045) on port 50010.

  CONFIRMED ON PS3, 2026-08-05: a real console accepted these datagrams through
  the reliable-UDP handshake and the boot sequence, and reached in-game play.

  Much simpler than the inbound direction - no auth token, no packet-type byte,
  and the AAD is just the IV. The server is replying to a known peer address, so
  none of the demux machinery is needed:

    inbound  : token(8) || iv(11) || tag(16) || ptype(1) || ct   AAD = iv||token||ptype
    outbound :             iv(11) || tag(16)             || ct   AAD = iv

  The server never sends a connection prefix.

seq:
  - id: iv
    size: 11
  - id: tag
    size: 16
  - id: ciphertext
    size-eos: true
    doc: AES-CWC-128. Decrypts directly to a `rudp_packet`.

types:
  rudp_packet:
    doc: |
      The opcodes the server emits are SYN_ACK and ACK during setup, DAT /
      DAT_ACK for data, HBT for heartbeats, FIN_ACK on teardown.
    seq:
      - id: magic0
        contents: [0xF5]
      - id: magic1
        contents: [0x02]
      - id: local_low
        type: u1
      - id: seq_high
        type: u1
        doc: high nibble = local bits 8-11; low nibble = remote bits 8-11.
      - id: remote_low
        type: u1
      - id: opcode
        type: u1
        enum: rudp_opcode
      - id: trailer
        contents: [0xFF]
      - id: payload
        size-eos: true
        doc: |
          SYN_ACK carries the fixed 8 bytes 12 10 20 20 00 01 00 00 - differing
          from the client's SYN payload only in bytes 5-6.
          DAT / DAT_ACK carry a `fragment`.

  fragment:
    seq:
      - id: packet_counter
        type: u2le
      - id: compress_flag
        type: u1
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
      Two kinds, distinguished only here:

        REPLY - msg_type 0, msg_index copied from the request, and a 16-byte
                response header before the protobuf.
        PUSH  - unsolicited. The PC reference says these use msg_type 0x0320 with
                msg_index 0xFFFFFFFF, identified by the first protobuf field.
                UNRESOLVED ON PS3: the client's push dispatcher (vaddr 0x158C138)
                keys on a u32 the caller has already placed on the stack, and
                decompilation could not establish statically whether that value
                comes from the transport header or from a parsed protobuf field.
                Both models fit the evidence. Do not assume the PC model holds
                here until a push has actually been driven end to end.

      IMPORTANT - 16 client->server opcodes register NO response callback, so the
      client never parses a reply body for them. Confirmed live: the client
      reached in-game while 0x03A8 and 0x03B8 went unanswered. The PC reference
      classifies most of those as request/response, which is wrong for PS3. The
      full list is in docs/protocol-map-ps3.md.
    seq:
      - id: header_size
        type: u4
        doc: Always 12.
      - id: msg_type
        type: u4
        doc: 0 for a reply. See the push caveat above.
      - id: msg_index
        type: u4le
        doc: Echoed from the request for a reply.
      - id: response_header
        type: response_header
        if: msg_type == 0
      - id: protobuf_body
        size-eos: true

  response_header:
    doc: |
      {0, 1, 0, 0} written LITTLE-ENDIAN here. The TCP response header holds the
      same values BIG-endian - they look identical in a hexdump only because the
      values are small. Do not share a codec between the two.
    seq:
      - id: unknown_0
        type: u4le
      - id: unknown_1
        type: u4le
      - id: unknown_2
        type: u4le
      - id: unknown_3
        type: u4le

enums:
  rudp_opcode:
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

  # Push opcodes the CLIENT registers handlers for, i.e. what the server may send.
  # Recovered by decompilation; the alias blocks are the biggest divergence from
  # the PC reference and are NOT individually resolved - see below.
  push_opcode:
    0x0389: push_special_0389        # special-cased ahead of the handler map
    0x038b: push_special_038b        # special-cased; PC calls this DS3-only
    0x038c: player_info_upload_config  # special-cased; pushed after login
    0x039b: push_request_summon_sign
    0x039c: push_request_reject_sign
    0x039d: push_request_remove_sign
    0x03a5: push_request_summon_mirror_knight_sign
    0x03a6: push_request_reject_mirror_knight_sign
    0x03a7: push_request_remove_mirror_knight_sign
    0x03aa: push_request_evaluate_blood_message
    # BreakIn registers SIXTEEN push handlers, 0x03B9-0x03C8: four message types
    # x four aliases each. The PC reference's 0x03FB/0x03FC/0x03FD DO NOT EXIST in
    # this client - a server using them would never have a push dispatched.
    # Which alias maps to which message type is UNKNOWN; every registration site
    # loads the same callback vtable and the distinguishing state is passed
    # through the callback object, invisible to static analysis. A single live
    # invasion capture would settle it.
    0x03b9: break_in_push_alias_0
    0x03ba: break_in_push_alias_1
    0x03bb: break_in_push_alias_2
    0x03bc: break_in_push_alias_3
    0x03bd: break_in_push_alias_4
    0x03be: break_in_push_alias_5
    0x03bf: break_in_push_alias_6
    0x03c0: break_in_push_alias_7
    0x03c1: break_in_push_alias_8
    0x03c2: break_in_push_alias_9
    0x03c3: break_in_push_alias_10
    0x03c4: break_in_push_alias_11
    0x03c5: break_in_push_alias_12
    0x03c6: break_in_push_alias_13
    0x03c7: break_in_push_alias_14
    0x03c8: break_in_push_alias_15
    # Visitor registers NINE, 0x03C9-0x03D1: three message types x three aliases.
    # The PC reference lists only the last three and claims 0x03C9 is an
    # unregistered bell push - on PS3 0x03C9 is the first Visitor alias.
    0x03c9: visitor_push_alias_0
    0x03ca: visitor_push_alias_1
    0x03cb: visitor_push_alias_2
    0x03cc: visitor_push_alias_3
    0x03cd: visitor_push_alias_4
    0x03ce: visitor_push_alias_5
    0x03cf: visitor_push_alias_6
    0x03d0: visitor_push_alias_7
    0x03d1: visitor_push_alias_8
    # QuickMatch registers EIGHT, 0x03E0-0x03E7: four message types x two aliases.
    # The PC reference lists only the four odd values.
    0x03e0: quick_match_push_alias_0
    0x03e1: quick_match_push_alias_1
    0x03e2: quick_match_push_alias_2
    0x03e3: quick_match_push_alias_3
    0x03e4: quick_match_push_alias_4
    0x03e5: quick_match_push_alias_5
    0x03e6: quick_match_push_alias_6
    0x03e7: quick_match_push_alias_7
    0x03ef: push_session_disconnect
