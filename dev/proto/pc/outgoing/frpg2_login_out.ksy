meta:
  id: frpg2_login_out
  title: Frpg2 login service - server to client (TCP)
  file-extension: bin
  endian: be
  ks-version: 0.9

doc: |
  One packet sent by the LOGIN service back to a Dark Souls 2/3 client over TCP.

  The login service emits exactly one kind of message: the reply to
  `request_query_login_server_info`, telling the client the auth server's address
  and port. After sending it the server waits for the client to disconnect.

  CRYPTO: the body is RSA encrypted with **X9.31** padding - NOT the OAEP used for
  the inbound direction. That asymmetry is why this spec is separate from
  frpg2_login_in.ksy rather than shared.

  Kaitai cannot decrypt, so `body` is ciphertext. Decrypt it, then parse the
  plaintext as `login_reply_message`.

  VERIFIED: byte-confirmed against a real DS2 PS3 client (BLUS41045), 2026-08-04.
  A real reply measured 298 bytes on the wire.

seq:
  - id: outer_len
    type: u2
    doc: Bytes following this field (12-byte header + body). Wire total = outer_len + 2.
  - id: header
    type: packet_header
  - id: body
    size: outer_len - 12
    doc: |
      RSA (X9.31) ciphertext. Decrypt, then parse as `login_reply_message`.

types:
  packet_header:
    doc: Frpg2PacketHeader, identical in both directions. All fields big-endian.
    seq:
      - id: send_counter
        type: u2
        doc: |
          Per-connection outbound counter. Note this counts the SERVER's sends and is
          independent of the client's counter - the two directions do not share a
          sequence space.
      - id: unknown_1
        type: u1
      - id: unknown_2
        type: u1
      - id: payload_length
        type: u4
        doc: Body length, i.e. outer_len - 12.
      - id: unknown_3
        type: u2
      - id: payload_length_short
        type: u2
        doc: Must equal payload_length.

  login_reply_message:
    doc: |
      Found inside the decrypted body. This is always a Reply, so unlike the inbound
      direction it carries the 16-byte response header between the message header and
      the protobuf.
    seq:
      - id: header_size
        type: u4
        doc: Always 12.
      - id: msg_type
        type: u4
        doc: |
          Always 0 (Reply). The reply does NOT echo the request's type - the only link
          back to the request is msg_index below.
      - id: msg_index
        type: u4le
        doc: LITTLE-ENDIAN. Copied verbatim from the request being answered.
      - id: response_header
        type: response_header
      - id: protobuf_body
        size-eos: true
        doc: |
          Protobuf `RequestQueryLoginServerInfoResponse`:
            required int64  port      = 1;   // auth service port (50000)
            required string server_ip = 2;   // auth service address
          The reference substitutes a private IP when the peer is on a private subnet,
          which matters on a LAN. Kaitai does not parse protobuf.

  response_header:
    doc: |
      Fixed 16-byte block present only on replies, big-endian {0, 1, 0, 0}. Its purpose
      is not understood; it is emitted verbatim and the client accepts it.

      Byte-order trap: the reliable-UDP message layer has a response header with the
      same {0,1,0,0} values but written LITTLE-endian. Here, on TCP, it is big-endian.
      The two are not interchangeable.
    seq:
      - id: unknown_0
        type: u4
        doc: Always 0.
      - id: unknown_1
        type: u4
        doc: Always 1.
      - id: unknown_2
        type: u4
        doc: Always 0.
      - id: unknown_3
        type: u4
        doc: Always 0.
