package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Connection keepalive and link-measurement opcodes.
//
// All four are request/response, so all four MUST be answered. An unanswered
// request/response opcode is not harmless on this client: it retries silently and
// will not open other online UI while one is outstanding, which is how several
// "broken menu" symptoms were eventually explained.
//
// ds3os does not implement any of these for DS2, so there is no reference
// behaviour to copy — but every one of the four messages, request and response
// alike, is defined with no fields at all. An empty message is therefore the
// complete and only possible reply, not a stub.
const (
	opServerPing                     uint32 = 0x038D
	opRequestMeasureUploadBandwidth  uint32 = 0x038E
	opRequestMeasureDownloadBandwith uint32 = 0x038F
	opRequestBenchmarkThroughput     uint32 = 0x03B7
)

// handleServerPing answers the client's liveness probe.
//
// Despite the name this is client->server: the client pings us. Leaving it
// unanswered is a candidate cause of the unexplained periodic disconnects, since
// a client that believes the server is gone has no reason to keep the session.
func (s *Service) handleServerPing(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.ServerPing
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse ServerPing: %w", err)
	}
	log.Debug("server ping", "player_id", cs.playerID)
	// ServerPing is its own response type: the client sends the same empty
	// message back and forth.
	return proto.Marshal(&ds2pb.ServerPing{})
}

// handleMeasureUploadBandwidth answers the client's upload measurement.
//
// The reply carries no payload, so the client can only be timing the round trip
// rather than measuring throughput of a body. If a future capture shows it
// expecting bulk data, that will appear as a repeated request rather than as an
// error.
func (s *Service) handleMeasureUploadBandwidth(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestMeasureUploadBandwidth
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestMeasureUploadBandwidth: %w", err)
	}
	log.Info("upload bandwidth measurement requested",
		"player_id", cs.playerID, "request_bytes", len(payload))
	return proto.Marshal(&ds2pb.RequestMeasureUploadBandwidthResponse{})
}

// handleMeasureDownloadBandwidth answers the client's download measurement. See
// handleMeasureUploadBandwidth for the empty-body caveat.
func (s *Service) handleMeasureDownloadBandwidth(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestMeasureDownloadBandwidth
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestMeasureDownloadBandwidth: %w", err)
	}
	log.Info("download bandwidth measurement requested",
		"player_id", cs.playerID, "request_bytes", len(payload))
	return proto.Marshal(&ds2pb.RequestMeasureDownloadBandwidthResponse{})
}

// handleBenchmarkThroughput answers the client's throughput benchmark.
func (s *Service) handleBenchmarkThroughput(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestBenchmarkThroughput
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestBenchmarkThroughput: %w", err)
	}
	log.Info("throughput benchmark requested",
		"player_id", cs.playerID, "request_bytes", len(payload))
	return proto.Marshal(&ds2pb.RequestBenchmarkThroughputResponse{})
}
