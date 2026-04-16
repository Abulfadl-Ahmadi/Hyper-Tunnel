package protocol

import "testing"

func TestHybridCapabilitiesCodecRoundTrip(t *testing.T) {
	want := HybridCapabilities{
		HybridSupported: true,
		MaxFeedbackRate: 120,
		MaxStreams:      1024,
	}

	encoded := EncodeHybridCapabilities(want)
	got, err := DecodeHybridCapabilities(encoded[:])
	if err != nil {
		t.Fatalf("DecodeHybridCapabilities returned error: %v", err)
	}

	if got != want {
		t.Fatalf("unexpected capabilities roundtrip: got=%+v want=%+v", got, want)
	}
}

func TestInitPacketWithHybridCapabilitiesRoundTrip(t *testing.T) {
	wantTarget := "example.target"
	wantCaps := HybridCapabilities{
		HybridSupported: true,
		MaxFeedbackRate: 90,
		MaxStreams:      300,
	}

	packet := NewInitPacketWithHybridCapabilities(9, wantTarget, wantCaps)
	if packet.Type != PacketInit {
		t.Fatalf("unexpected packet type: got=%d want=%d", packet.Type, PacketInit)
	}

	target, caps, hasCaps, err := ParseInitWithHybridCapabilities(packet.Payload)
	if err != nil {
		t.Fatalf("ParseInitWithHybridCapabilities returned error: %v", err)
	}
	if !hasCaps {
		t.Fatal("expected capability block to be present")
	}
	if target != wantTarget {
		t.Fatalf("unexpected parsed target: got=%q want=%q", target, wantTarget)
	}
	if caps != wantCaps {
		t.Fatalf("unexpected parsed capabilities: got=%+v want=%+v", caps, wantCaps)
	}
}

func TestInitPacketWithoutHybridCapabilitiesStillParses(t *testing.T) {
	packet := NewInitPacket(7, "legacy.target")
	target, caps, hasCaps, err := ParseInitWithHybridCapabilities(packet.Payload)
	if err != nil {
		t.Fatalf("ParseInitWithHybridCapabilities returned error: %v", err)
	}
	if target != "legacy.target" {
		t.Fatalf("unexpected target: got=%q want=%q", target, "legacy.target")
	}
	if hasCaps {
		t.Fatal("did not expect capability block for legacy init payload")
	}
	if caps.HybridSupported || caps.MaxFeedbackRate != 0 || caps.MaxStreams != 0 {
		t.Fatalf("unexpected capability defaults: %+v", caps)
	}
}
