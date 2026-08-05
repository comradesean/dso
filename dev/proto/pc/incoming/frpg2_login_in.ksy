meta:
  id: frpg2_login_in
  title: Frpg2 login service - client to server (TCP)
  file-extension: bin
  endian: be
  ks-version: 0.9

doc: |
  One packet sent by a Dark Souls 2/3 client to the LOGIN service over TCP.

  The login service answers exactly one question - "where do I authenticate?" - and
  disconnects on anything else. The client sends `request_query_login_server_info`
  (msg_type 5) and receives a reply carrying the auth server's address.

  Ports: 50011 on PS3 (BLUS41045), 50050 on PC.

  CRYPTO: the whole message body is RSA encrypted. The server decrypts inbound with
  RSA/PKCS#1 **OAEP** and encrypts its replies with RSA **X9.31** - the padding differs
  by direction, so this is not symmetric with frpg2_login_out.ksy.

  Because Kaitai cannot decrypt, `body` below is the RSA ciphertext. Decoding is a
  two-pass job: parse this envelope, OAEP-decrypt `body`, then parse the plaintext as
  `login_message` (defined here) and finally the protobuf inside it.

  VERIFIED: this framing is byte-confirmed against a real DS2 PS3 client (BLUS41045),
  captured 2026-08-04.

seq:
  - id: outer_len
    type: u2
    doc: |
      Length of everything after this field: the 12-byte packet header plus the body.
      Total bytes on the wire = outer_len + 2. Bounded by MAX_PACKET_LENGTH (64 KiB).
  - id: header
    type: packet_header
  - id: body
    size: outer_len - 12
    doc: |
      RSA (OAEP) ciphertext. Decrypt, then parse the plaintext as `login_message`.
      Observed 282 bytes on the wire for a real PS3 login: 2 + 12 + 268, where the
      268-byte body is a 256-byte RSA-2048 block plus the 12-byte message header
      that shares the encrypted region.

types:
  packet_header:
    doc: |
      Frpg2PacketHeader. All fields big-endian. Note that the payload length is
      carried TWICE, as a u4 and again as a u2; our implementation validates that
      both agree with the outer length and rejects the packet otherwise, which is a
      cheap way to catch a desynchronised stream.
    seq:
      - id: send_counter
        type: u2
        doc: Per-connection outbound packet counter, incremented before each send.
      - id: unknown_1
        type: u1
        doc: Always 0 in every capture.
      - id: unknown_2
        type: u1
        doc: Always 0 in every capture.
      - id: payload_length
        type: u4
        doc: Body length, i.e. outer_len - 12.
      - id: unknown_3
        type: u2
        doc: Always 0 in every capture.
      - id: payload_length_short
        type: u2
        doc: The same length again as a u2. Must equal payload_length.

  login_message:
    doc: |
      The message layer, found inside the decrypted body. A client->server login
      message is never a Reply, so it has no response header - the protobuf begins
      immediately after the 12-byte header.
    seq:
      - id: header_size
        type: u4
        doc: Always 12.
      - id: msg_type
        type: u4
        enum: msg_type
        doc: For the login service this is always request_query_login_server_info (5).
      - id: msg_index
        type: u4le
        doc: |
          LITTLE-ENDIAN, unlike every other multi-byte field here. The server's reply
          copies this value verbatim, and that is the only way to associate a reply
          with its request - the reply's own msg_type is 0 (Reply), not the request's
          type. Getting this byte order wrong is an easy and very confusing bug.
      - id: protobuf_body
        size-eos: true
        doc: |
          Protobuf `RequestQueryLoginServerInfo`:
            required string steam_id    = 1;  // on PS3 this carries the PSN online ID
            optional string f2          = 2;
            required uint64 app_version = 3;  // PS3 DS2 = 16912640 (0x1020000)
                                              // PC SOTFS = 17039619
          Despite the field name, `steam_id` is a generic platform account id. Kaitai
          does not parse protobuf; use the .proto definitions.

enums:
  msg_type:
    0: reply
    1: key_material
    2: get_service_status
    3: platform_ticket
    5: request_query_login_server_info
    6: request_handshake
