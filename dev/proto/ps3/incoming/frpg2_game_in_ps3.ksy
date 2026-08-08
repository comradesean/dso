meta:
  id: frpg2_game_in_ps3
  title: Frpg2 game service - client to server, PS3 (reliable UDP)
  file-extension: bin
  endian: be
  ks-version: 0.9

doc: |
  One UDP datagram sent by a DARK SOULS 2 PS3 client (BLUS41045) to the game
  service on port 50010.

  CONFIRMED ON PS3, 2026-08-05. Unlike the pc/ specs, the framing here has been
  driven end to end by a real console: the reliable-UDP handshake reaches
  Established and the client plays in-game against this server.

  The opcode enum below is recovered by DECOMPILATION of the retail EBOOT, not
  from the PC reference, and it CONTRADICTS that reference in several places.
  Where the two disagree, this file wins for PS3. See docs/protocol-map-ps3.md.

  Four nested layers, peeled in order:

    1. datagram envelope   token, IV, tag, packet-type byte  (top level here)
    2. reliable-UDP packet 7-byte header                     (`rudp_packet`)
    3. fragment            12-byte header, reassembly        (`fragment`)
    4. message             12-byte header + protobuf         (`game_message`)

seq:
  - id: auth_token
    size: 8
    doc: |
      The 8 bytes issued by the auth service in the 56-byte game_server_info,
      sent IN THE CLEAR on every client->server datagram. The server looks this
      up to find the session's CWC key before it can decrypt anything.
  - id: iv
    size: 11
  - id: tag
    size: 16
  - id: packet_type
    type: u1
    enum: packet_type
    doc: 1 on the SYN datagram whose plaintext carries the connection prefix, else 0.
  - id: ciphertext
    size-eos: true
    doc: |
      AES-CWC-128. AAD is `iv || auth_token || packet_type` for this direction -
      not just the IV. Decrypts to an optional 35-byte `connection_prefix`
      followed by a `rudp_packet`.

types:
  connection_prefix:
    doc: |
      35 bytes, present only on the SYN datagram. Carries the platform account id
      twice - on PS3 the PSN online ID. Client-only; the server never sends one.
    seq:
      - id: id_first
        size: 17
        type: strz
        encoding: ASCII
      - id: separator
        type: u1
      - id: id_second
        size: 17
        type: strz
        encoding: ASCII

  rudp_packet:
    doc: |
      CONFIRMED against the console. Sequence numbers are 12-bit, wrapping at
      4096, packed across three bytes.

      Only DAT, DAT_ACK and FIN_ACK consume a sequence number and enter the
      retransmit queue. Note the session does NOT reach Established on the first
      SYN_ACK - both peers pass through SynReceived and the following ACK
      exchange completes it.
    seq:
      - id: magic0
        contents: [0xF5]
      - id: magic1
        contents: [0x02]
      - id: local_low
        type: u1
      - id: seq_high
        type: u1
        doc: |
          high nibble = bits 8-11 of local; low nibble = bits 8-11 of remote.
            local  = local_low  | ((seq_high >> 4)  << 8)
            remote = remote_low | ((seq_high & 0x0F) << 8)
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
          SYN carries the fixed 8 bytes 12 10 20 20 00 00 A0 00.
          DAT / DAT_ACK carry a `fragment`. ACK / HBT are usually empty.

  fragment:
    seq:
      - id: packet_counter
        type: u2le
        doc: LITTLE-ENDIAN, unlike the rest of this header.
      - id: compress_flag
        type: u1
        doc: |
          Non-zero if the reassembled payload is zlib-compressed; fragment 0 then
          carries an extra u4be decompressed length before its body.

          The stream uses a 4 KB window (CINFO=5), so it begins `58 c3` rather
          than the familiar `78 9c`. That is a valid zlib header - CM=8, and the
          two bytes divide by 31 - but it does not look like one at a glance.
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
      Concatenate fragment bodies in index order (then zlib inflate if flagged)
      to get this.
    seq:
      - id: header_size
        type: u4
        doc: Always 12.
      - id: msg_type
        type: u4
        enum: opcode
      - id: msg_index
        type: u4le
        doc: |
          LITTLE-ENDIAN. RESOLVED 2026-08-07 - the two fields before it are
          big-endian and this one is not, which is as odd as it looks.

          Proof: across a 15,573-message corpus, every message whose payload was
          mistakenly taken to start at offset 8 had its first four bytes equal
          the message index read as a little-endian u32. 6,515 of 6,515, zero
          mismatches.

          This is NOT cosmetic, and assuming otherwise cost real work: a reply
          echoes the index of the request it answers, so it is the only thing
          that pairs a response with its request. Half a capture corpus is
          responses, whose header carries no opcode at all, and they are
          anonymous without it.
      - id: protobuf_body
        size-eos: true
        doc: |
          Protobuf; Kaitai does not parse it. See docs/protocol-map-ps3.md.

          The body starts at offset 12 for a request, and NEVER at 8 - offset 8
          is the msg_index above. Four bytes of index parse as valid protobuf
          often enough to fool a decoder that probes offsets, which is exactly
          what happened to cmd/corpus: 42% of its output carried the index
          prepended to the payload and a decoded tree built from it.

          The body may contain protobuf GROUPS (wire types 3 and 4), the
          deprecated construct. DS2 still uses them - RequestNotifyKillEnemy and
          the RequestGetRightMatchingArea response both do - so a reader that
          rejects wire type 3 will stop mid-message on live traffic.

enums:
  packet_type:
    0: normal
    1: connection_prefix

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

  # The PS3 opcode space: 0x0320 plus 0x0386-0x03F9 on the LAUNCH DISC.
  #
  # CORRECTED 2026-08-07: 0x03FA DOES exist, in v1.10 only. `li r4,0x03fa`
  # occurs zero times in the v1.00 EBOOT and twice in the title update, and two
  # real v1.10 clients were seen sending it at boot. It is implemented. The
  # never-implement list is therefore FIVE opcodes - 0x03FB, 0x03FC, 0x03FD,
  # 0x03FF, 0x0400 - not six.
  #
  # The lesson generalises: "absent from the binary" is only ever true of the
  # build it was measured on.
  #
  # Names marked LIVE were driven by a real BLUS41045 client against this server
  # on 2026-08-05. The rest are decomp-derived.
  opcode:
    0x0320: request_send_message_to_players    # client-relayed push tunnel
    0x0386: request_wait_for_user_login        # LIVE - first message, returns player_id
    0x0387: unknown_0387                       # exists on PS3, in neither PC table
    # 0x0387/0x0388/0x038A are CLIENT-TO-SERVER, not pushes: each has a send site
    # (0x16638F4, 0x1663994, 0x1663A34, all from one function 0x16633A8) rather
    # than a push-handler registration. None has ever been observed on the wire,
    # and they cannot appear in a PC capture because they are PS3-only. The best
    # candidate for what switches them on is 0x038C, which carries exactly three
    # upload periods against exactly three opcodes from one dispatch function.
    0x0388: unknown_0388                       # exists on PS3, in neither PC table
    0x038a: unknown_038a                       # exists on PS3, in neither PC table
    0x038d: server_ping                        # PC reference calls this DS3-only
    0x038e: request_measure_upload_bandwidth   # PC reference calls this DS3-only
    0x038f: request_measure_download_bandwidth # PC reference calls this DS3-only
    0x0390: unknown_0390                       # NRLogging-related
    0x0391: request_create_bloodstain          # no response callback
    0x0392: request_get_bloodstain_list
    0x0393: request_get_deading_ghost
    0x0394: request_create_sign
    0x0395: request_update_sign
    0x0396: request_remove_sign
    0x0397: request_get_sign_list
    0x0398: request_summon_sign
    0x039a: request_reject_sign
    0x039e: request_create_mirror_knight_sign
    0x039f: request_update_mirror_knight_sign
    0x03a0: request_remove_mirror_knight_sign
    0x03a1: request_get_mirror_knight_sign_list
    0x03a2: request_summon_mirror_knight_sign
    0x03a4: request_reject_mirror_knight_sign
    0x03a8: request_update_player_character     # LIVE - no response callback
    0x03a9: request_get_player_character
    0x03ab: request_create_blood_message
    0x03ac: request_remove_blood_message
    0x03ad: request_reentry_blood_message
    0x03ae: request_get_blood_message_list
    0x03af: request_evaluate_blood_message
    0x03b0: request_get_blood_message_evaluation
    0x03b1: request_create_ghost_data
    0x03b2: request_get_ghost_data_list         # LIVE
    0x03b3: request_get_login_player_character
    0x03b5: request_get_player_character_list   # PC calls DS3-only, and at 0x03a1
    0x03b6: request_update_login_player_character  # LIVE - allocates character_id
    0x03b7: request_benchmark_throughput        # PC calls DS3-only, and at 0x03a3
    0x03b8: request_update_player_status        # LIVE - no response callback
    0x03d2: request_get_break_in_target_list
    0x03d3: request_break_in_target
    0x03d4: request_reject_break_in_target
    0x03d5: request_get_visitor_list
    0x03d6: request_visit
    0x03d7: request_reject_visit
    0x03d8: request_notify_mirror_knight        # no response callback
    0x03d9: request_register_quick_match
    0x03da: request_unregister_quick_match
    0x03db: request_update_quick_match
    0x03dc: request_search_quick_match
    0x03dd: request_join_quick_match
    0x03de: request_reject_quick_match
    0x03e8: request_notify_join_guest_player    # no response callback
    0x03e9: request_notify_leave_guest_player   # no response callback
    0x03ea: request_notify_join_session         # no response callback
    0x03eb: request_notify_leave_session        # no response callback
    0x03ec: request_get_announce_message_list   # LIVE - "Retrieving information"
    0x03ed: request_notify_kill_player          # no response callback
    0x03ee: request_notify_ring_bell            # no response callback
    0x03f0: request_get_total_death_count
    0x03f1: request_notify_death                # no response callback
    0x03f2: request_notify_offline_death_count  # no response callback
    0x03f3: request_register_power_stone_data
    0x03f4: request_get_power_stone_ranking
    0x03f5: request_get_power_stone_my_ranking
    0x03f6: request_notify_kill_enemy           # no response callback
    0x03f7: request_notify_buy_item             # no response callback
    0x03f8: request_get_power_stone_ranking_record_count
    0x03f9: request_notify_disconnect_session   # no response callback
    # v1.10 ONLY -- absent from the launch disc, present in the title update, and
    # seen at boot from two real v1.10 clients. Feeds the bonfire warp screen's
    # population hints; the response is repeated protobuf GROUPS of
    # {area_id, population}, three of them, sorted descending.
    0x03fa: request_get_right_matching_area     # LIVE (v1.10)
