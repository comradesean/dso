meta:
  id: frpg2_auth_in
  title: Frpg2 auth service - client to server (TCP)
  file-extension: bin
  endian: be
  ks-version: 0.9

doc: |
  One packet sent by a Dark Souls 2/3 client to the AUTH service over TCP (port 50000).

  The auth service runs a strictly sequential four-stage handshake. Any message
  arriving out of order disconnects the client:

    stage 1  request_handshake    (6)  -> negotiates the AES-CWC session key
    stage 2  get_service_status   (2)  -> app-version gate + identity
    stage 3  key_material         (1)  -> derives the UDP game-session key
    stage 4  platform_ticket      (3)  -> validates the ticket, returns game server info

  CRYPTO CHANGES MID-STREAM, which is the thing to watch when decoding a capture:

    * Stage 1's body is RSA (OAEP inbound).
    * The server's stage-1 REPLY is sent in the CLEAR (see frpg2_auth_out.ksy).
    * From stage 2 onward every body is AES-CWC-128, framed as
      `iv(11) || tag(16) || ciphertext`, with the IV also serving as the
      authenticated header (AAD). The key is the one the client supplied in stage 1.

  So a single capture contains three different body encodings. Parse the envelope
  here, then decrypt according to the stage.

  VERIFIED: byte-confirmed against a real DS2 PS3 client (BLUS41045), 2026-08-04/05.
  Observed wire sizes for a full successful handshake: 282, 73, 61, 329.

seq:
  - id: outer_len
    type: u2
    doc: Bytes following this field. Wire total = outer_len + 2.
  - id: header
    type: packet_header
  - id: body
    size: outer_len - 12
    doc: |
      Stage 1: RSA (OAEP) ciphertext.
      Stages 2-4: `cwc_envelope` (defined below).

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
    doc: |
      The AES-CWC-128 framing used from stage 2 onward, and identically by the TCP
      cipher in both directions.

      The tag is a Carter-Wegman polynomial hash over GF(2^127-1), NOT a standard AEAD
      construction, and the reference implementation's variant differs from the
      published CWC test vectors - matching the published vectors produces tags the
      game rejects. See internal/crypto/cwc for the details; this cost a full debugging
      session to discover.

      The plaintext recovered from `ciphertext` is an `auth_message`.
    seq:
      - id: iv
        size: 11
        doc: |
          Random per message. Also used verbatim as the authenticated header (AAD) for
          the tag - it is not just a nonce.
      - id: tag
        size: 16
        doc: CWC authentication tag over (iv as AAD, ciphertext).
      - id: ciphertext
        size-eos: true
        doc: |
          AES-CTR keystream, counter block `0x80 || iv(11) || u4be counter`, first data
          block using counter 1. Decrypts to an `auth_message`.

  auth_message:
    doc: |
      The message layer. Client->server auth messages are never replies, so there is no
      response header - the body starts right after the 12-byte header.

      NOTE that only stage 2's body is protobuf. Stages 1, 3 and 4 carry raw
      structures, which is unusual for this protocol and easy to trip over.
    seq:
      - id: header_size
        type: u4
        doc: Always 12.
      - id: msg_type
        type: u4
        enum: msg_type
      - id: msg_index
        type: u4le
        doc: LITTLE-ENDIAN. The server's reply copies this value.
      - id: body
        size-eos: true
        doc: |
          Shape depends on msg_type:
            request_handshake  -> `handshake_request` (protobuf, one bytes field)
            get_service_status -> protobuf `GetServiceStatus`
            key_material       -> `key_material_request` (RAW 8 bytes, not protobuf)
            platform_ticket    -> `ticket_request` (RAW, not protobuf)

  handshake_request:
    doc: |
      Protobuf `RequestHandshake`: `required bytes aes_cwc_key = 1;`
      The 16-byte AES-CWC key protecting the REST OF THIS TCP CONNECTION only. The UDP
      game session uses a different key, derived in stage 3.
    seq:
      - id: protobuf_body
        size-eos: true

  key_material_request:
    doc: |
      Stage 3 body. RAW BYTES, not protobuf. Exactly 8 bytes; any other length
      disconnects the client. These become the low half of the 16-byte CWC key used
      for the entire subsequent UDP game session, the server supplying the high half.
    seq:
      - id: client_half
        size: 8

  ticket_request:
    doc: |
      Stage 4 body. RAW BYTES, not protobuf.

      On PS3 the ticket is a **PSN NP ticket**, not a Steam ticket, despite the message
      type's Steam-flavoured name. Its internal format has not been captured or parsed.
      A real PS3 client sent a 329-byte packet at this stage.
    seq:
      - id: game_cwc_key_echo
        size: 16
        doc: The 16-byte game session key from stage 3, echoed back by the client.
      - id: ticket
        size-eos: true
        doc: Platform ticket - PSN NP ticket on PS3, Steam GetAuthSessionTicket on PC.

enums:
  msg_type:
    0: reply
    1: key_material
    2: get_service_status
    3: platform_ticket
    5: request_query_login_server_info
    6: request_handshake
