meta:
  id: frpg2_auth_out
  title: Frpg2 auth service - server to client (TCP)
  file-extension: bin
  endian: be
  ks-version: 0.9

doc: |
  One packet sent by the AUTH service back to a Dark Souls 2/3 client (port 50000).

  Every message here is a Reply (msg_type 0) echoing the request's msg_index. There
  are four, one per handshake stage:

    stage 1 reply -> `handshake_reply_blob`   27 raw bytes, sent IN THE CLEAR
    stage 2 reply -> protobuf GetServiceStatusResponse
    stage 3 reply -> `key_material_reply`     16 raw bytes
    stage 4 reply -> `game_server_info`       184 raw bytes

  THE STAGE-1 REPLY IS UNENCRYPTED. The request that preceded it was RSA-encrypted,
  and every later message is AES-CWC, but this one datagram is plaintext: the cipher
  is deliberately detached to send it and the CWC cipher installed immediately after.
  A decoder that assumes uniform encryption will fail exactly here.

  From stage 2 onward bodies use the same `cwc_envelope` as the inbound direction
  (iv 11 || tag 16 || ciphertext), keyed by the client-supplied stage-1 key.

  VERIFIED: byte-confirmed against a real DS2 PS3 client (BLUS41045), 2026-08-05.
  Observed wire sizes for a successful handshake: 69, 80, 85, 253.

seq:
  - id: outer_len
    type: u2
  - id: header
    type: packet_header
  - id: body
    size: outer_len - 12
    doc: |
      Stage 1: PLAINTEXT `auth_reply_message` (no cipher).
      Stages 2-4: `cwc_envelope`, decrypting to an `auth_reply_message`.

types:
  packet_header:
    seq:
      - id: send_counter
        type: u2
      - id: unknown_1
        type: u1
      - id: unknown_2
        type: u1
      - id: payload_length
        type: u4
      - id: unknown_3
        type: u2
      - id: payload_length_short
        type: u2

  cwc_envelope:
    doc: See frpg2_auth_in.ksy for the full cipher notes; the framing is identical.
    seq:
      - id: iv
        size: 11
      - id: tag
        size: 16
      - id: ciphertext
        size-eos: true

  auth_reply_message:
    doc: |
      Every auth reply carries the 16-byte response header, so the body begins 28 bytes
      in (12 header + 16 response header) rather than 12.

      Worked example, the 69-byte stage-1 reply on the wire:
        2   outer_len   = 0x0043 (67)
        12  packet header (payload_length = 55)
        12  message header (type 0, index copied)
        16  response header {0,1,0,0}
        27  handshake_reply_blob
        = 69 bytes
    seq:
      - id: header_size
        type: u4
        doc: Always 12.
      - id: msg_type
        type: u4
        doc: Always 0 (Reply).
      - id: msg_index
        type: u4le
        doc: LITTLE-ENDIAN. Copied from the request being answered.
      - id: response_header
        type: response_header
      - id: body
        size-eos: true
        doc: |
          Shape depends on which stage is being answered:
            stage 1 -> handshake_reply_blob (27 raw bytes)
            stage 2 -> protobuf GetServiceStatusResponse
            stage 3 -> key_material_reply (16 raw bytes)
            stage 4 -> game_server_info (184 raw bytes)

  response_header:
    doc: Big-endian {0, 1, 0, 0}. The UDP equivalent is little-endian - do not share code.
    seq:
      - id: unknown_0
        type: u4
      - id: unknown_1
        type: u4
      - id: unknown_2
        type: u4
      - id: unknown_3
        type: u4

  handshake_reply_blob:
    doc: |
      Stage 1 reply. 27 raw bytes, PLAINTEXT.

      The reference implementation calls this payload mysterious. It is not: 11 is the
      CWC IV length and 16 is the CWC tag length, so the server is declaring its cipher
      framing parameters to the client. The 11 random bytes appear to be ignored.
    seq:
      - id: random
        size: 11
        doc: Random. Equal to the CWC IV length.
      - id: zeros
        size: 16
        doc: All zero. Equal to the CWC tag length.

  key_material_reply:
    doc: |
      Stage 3 reply. 16 raw bytes, and the most important value in the handshake: this
      is the AES-CWC key for the ENTIRE UDP game session. It is registered server-side
      against the auth token issued in stage 4.
    seq:
      - id: client_half
        size: 8
        doc: The client's 8 bytes from its stage-3 request, echoed back.
      - id: server_half
        size: 8
        doc: 8 server-random bytes.

  game_server_info:
    doc: |
      Stage 4 reply. 184 raw bytes, big-endian. Tells the client where the UDP game
      server is and issues the auth token that authorises it.

      *** UNCONFIRMED ON PS3 - THIS IS THE CURRENT PRIME SUSPECT. ***

      This layout is taken from the DS3OS reference, which targets PC/SOTFS. Against a
      real PS3 client it does NOT work: the client completes all four auth stages,
      reads this struct, closes the auth socket (normal), and then refuses to use its
      already-bound UDP socket - it never connects or sends. It is rejecting something
      in here. A decompilation of the PS3 EBOOT is underway to recover the real layout.
      Do not trust these offsets for PS3.
    seq:
      - id: auth_token
        size: 8
        doc: |
          Random. NOT byte-swapped - raw bytes. The client prefixes this to every
          client->server game datagram in the clear, which is how a connectionless
          UDP listener demuxes sessions before it can decrypt anything.
      - id: game_server_ip
        size: 16
        type: strz
        encoding: ASCII
        doc: NUL-terminated ASCII address, e.g. "192.168.1.100".
      - id: stack_data
        size: 112
        doc: |
          Zeroed. The retail server reportedly leaked stack memory here, so the
          contents are presumed ignored - but that is an assumption, and if the PS3
          client validates anything in this region it would explain the rejection.
      - id: game_port
        type: u2
        doc: UDP game service port, big-endian (50010).
      - id: padding
        type: u2
        doc: Zero.
      - id: tuning
        type: u4
        repeat: expr
        repeat-expr: 11
        doc: |
          Eleven big-endian u4 constants, believed to be buffer sizes:
            0x8000, 0x8000, 0xA000, 0xA000, 0x80, 0x8000, 0xA000,
            0x493E0, 0x61A8, 0x0C, 0x00
          Their meaning is unverified in either implementation.
