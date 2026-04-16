package enums

import "testing"

func TestControlAckForHybridPackets(t *testing.T) {
	tests := map[uint8]uint8{
		PACKET_HYBRID_STREAM_OPEN:  PACKET_HYBRID_STREAM_OPEN_ACK,
		PACKET_HYBRID_STREAM_CLOSE: PACKET_HYBRID_STREAM_CLOSE_ACK,
		PACKET_HYBRID_STREAM_RESET: PACKET_HYBRID_STREAM_RESET_ACK,
	}

	for packetType, wantAck := range tests {
		ack, ok := ControlAckFor(packetType)
		if !ok {
			t.Fatalf("expected control ack mapping for packet type %d", packetType)
		}
		if ack != wantAck {
			t.Fatalf("unexpected ack for packet type %d: got=%d want=%d", packetType, ack, wantAck)
		}

		reverse, ok := ReverseControlAckFor(ack)
		if !ok {
			t.Fatalf("expected reverse ack mapping for ack packet type %d", ack)
		}
		if reverse != packetType {
			t.Fatalf("unexpected reverse mapping for ack %d: got=%d want=%d", ack, reverse, packetType)
		}
	}
}

func TestGetPacketCloseStreamIncludesHybridCloseReset(t *testing.T) {
	tests := map[uint8]uint8{
		PACKET_HYBRID_STREAM_CLOSE: PACKET_HYBRID_STREAM_CLOSE_ACK,
		PACKET_HYBRID_STREAM_RESET: PACKET_HYBRID_STREAM_RESET_ACK,
	}

	for packetType, wantAck := range tests {
		ack, ok := GetPacketCloseStream(packetType)
		if !ok {
			t.Fatalf("expected close-stream mapping for packet type %d", packetType)
		}
		if ack != wantAck {
			t.Fatalf("unexpected close-stream ack for packet type %d: got=%d want=%d", packetType, ack, wantAck)
		}
	}
}
