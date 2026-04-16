package vpnproto

import "testing"

func TestSessionAcceptClientPolicyRoundTrip(t *testing.T) {
	policy := SessionAcceptClientPolicy{
		MaxPacketDuplicationCount: 5,
		MaxSetupDuplicationCount:  6,
		MaxUploadMTU:              150,
		MaxDownloadMTU:            4000,
		MaxRxTxWorkers:            255,
		MinPingAggressiveInterval: 0.05,
		MaxPacketsPerBatch:        10,
		MaxARQWindowSize:          8000,
		MaxARQDataNackMaxGap:      128,
		MinCompressionMinSize:     120,
		MinARQInitialRTOSeconds:   0.25,
	}

	payload := EncodeSessionAcceptClientPolicy(policy)
	decoded, err := DecodeSessionAcceptClientPolicy(payload[:])
	if err != nil {
		t.Fatalf("DecodeSessionAcceptClientPolicy returned error: %v", err)
	}

	if decoded.MaxPacketDuplicationCount != policy.MaxPacketDuplicationCount {
		t.Fatalf("unexpected packet duplication: got=%d want=%d", decoded.MaxPacketDuplicationCount, policy.MaxPacketDuplicationCount)
	}
	if decoded.MaxSetupDuplicationCount != policy.MaxSetupDuplicationCount {
		t.Fatalf("unexpected setup duplication: got=%d want=%d", decoded.MaxSetupDuplicationCount, policy.MaxSetupDuplicationCount)
	}
	if decoded.MaxUploadMTU != policy.MaxUploadMTU {
		t.Fatalf("unexpected upload mtu: got=%d want=%d", decoded.MaxUploadMTU, policy.MaxUploadMTU)
	}
	if decoded.MaxDownloadMTU != policy.MaxDownloadMTU {
		t.Fatalf("unexpected download mtu: got=%d want=%d", decoded.MaxDownloadMTU, policy.MaxDownloadMTU)
	}
	if decoded.MaxRxTxWorkers != policy.MaxRxTxWorkers {
		t.Fatalf("unexpected workers: got=%d want=%d", decoded.MaxRxTxWorkers, policy.MaxRxTxWorkers)
	}
	if decoded.MinPingAggressiveInterval < 0.049 || decoded.MinPingAggressiveInterval > 0.051 {
		t.Fatalf("unexpected ping interval: got=%f", decoded.MinPingAggressiveInterval)
	}
	if decoded.MaxPacketsPerBatch != policy.MaxPacketsPerBatch {
		t.Fatalf("unexpected packets per batch: got=%d want=%d", decoded.MaxPacketsPerBatch, policy.MaxPacketsPerBatch)
	}
	if decoded.MaxARQWindowSize != policy.MaxARQWindowSize {
		t.Fatalf("unexpected arq window: got=%d want=%d", decoded.MaxARQWindowSize, policy.MaxARQWindowSize)
	}
	if decoded.MaxARQDataNackMaxGap != policy.MaxARQDataNackMaxGap {
		t.Fatalf("unexpected arq data nack gap: got=%d want=%d", decoded.MaxARQDataNackMaxGap, policy.MaxARQDataNackMaxGap)
	}
	if decoded.MinCompressionMinSize != policy.MinCompressionMinSize {
		t.Fatalf("unexpected compression min size: got=%d want=%d", decoded.MinCompressionMinSize, policy.MinCompressionMinSize)
	}
	if decoded.MinARQInitialRTOSeconds < 0.245 || decoded.MinARQInitialRTOSeconds > 0.255 {
		t.Fatalf("unexpected arq initial rto: got=%f", decoded.MinARQInitialRTOSeconds)
	}
}

func TestSessionAcceptPayloadRoundTrip(t *testing.T) {
	payload := SessionAcceptPayload{
		SessionID:       7,
		SessionCookie:   11,
		CompressionPair: 3,
		VerifyCode:      [4]byte{1, 2, 3, 4},
		NegotiatedHybridCapabilities: SessionHybridCapabilities{
			HybridSupported: true,
			MaxFeedbackRate: 100,
			MaxStreams:      500,
		},
		HasHybridCapabilitySync: true,
		ClientPolicy: SessionAcceptClientPolicy{
			MaxPacketDuplicationCount: 5,
			MaxSetupDuplicationCount:  6,
			MaxUploadMTU:              150,
			MaxDownloadMTU:            4096,
			MaxRxTxWorkers:            32,
			MinPingAggressiveInterval: 0.10,
			MaxPacketsPerBatch:        10,
			MaxARQWindowSize:          4096,
			MaxARQDataNackMaxGap:      64,
			MinCompressionMinSize:     120,
			MinARQInitialRTOSeconds:   0.20,
		},
		HasClientPolicySync: true,
	}

	encoded := EncodeSessionAcceptPayload(payload)
	decoded, err := DecodeSessionAcceptPayload(encoded)
	if err != nil {
		t.Fatalf("DecodeSessionAcceptPayload returned error: %v", err)
	}

	if decoded.SessionID != payload.SessionID || decoded.SessionCookie != payload.SessionCookie {
		t.Fatalf("unexpected decoded identity: got=(%d,%d) want=(%d,%d)", decoded.SessionID, decoded.SessionCookie, payload.SessionID, payload.SessionCookie)
	}
	if decoded.CompressionPair != payload.CompressionPair {
		t.Fatalf("unexpected compression pair: got=%d want=%d", decoded.CompressionPair, payload.CompressionPair)
	}
	if decoded.VerifyCode != payload.VerifyCode {
		t.Fatalf("unexpected verify code: got=%v want=%v", decoded.VerifyCode, payload.VerifyCode)
	}
	if !decoded.HasClientPolicySync {
		t.Fatal("expected client policy sync payload to round-trip")
	}
	if !decoded.HasHybridCapabilitySync {
		t.Fatal("expected hybrid capability sync payload to round-trip")
	}
	if !decoded.NegotiatedHybridCapabilities.HybridSupported {
		t.Fatal("expected negotiated hybrid support to be true")
	}
	if decoded.NegotiatedHybridCapabilities.MaxFeedbackRate != payload.NegotiatedHybridCapabilities.MaxFeedbackRate {
		t.Fatalf("unexpected negotiated max feedback rate: got=%d want=%d", decoded.NegotiatedHybridCapabilities.MaxFeedbackRate, payload.NegotiatedHybridCapabilities.MaxFeedbackRate)
	}
	if decoded.NegotiatedHybridCapabilities.MaxStreams != payload.NegotiatedHybridCapabilities.MaxStreams {
		t.Fatalf("unexpected negotiated max streams: got=%d want=%d", decoded.NegotiatedHybridCapabilities.MaxStreams, payload.NegotiatedHybridCapabilities.MaxStreams)
	}
}

func TestParseSessionInitHybridCapabilitiesRoundTrip(t *testing.T) {
	base := make([]byte, SessionInitPayloadBaseSize)
	base[0] = 1
	copy(base[6:10], []byte{1, 2, 3, 4})

	requested := SessionHybridCapabilities{
		HybridSupported: true,
		MaxFeedbackRate: 150,
		MaxStreams:      1200,
	}
	payload := AppendSessionInitHybridCapabilities(base, requested)

	decoded, has, err := ParseSessionInitHybridCapabilities(payload)
	if err != nil {
		t.Fatalf("ParseSessionInitHybridCapabilities returned error: %v", err)
	}
	if !has {
		t.Fatal("expected hybrid capability block to be present")
	}
	if decoded != requested {
		t.Fatalf("unexpected decoded hybrid capability: got=%+v want=%+v", decoded, requested)
	}
}

func TestSessionAcceptPayloadDecodeCapabilityWithoutPolicy(t *testing.T) {
	payload := SessionAcceptPayload{
		SessionID:       9,
		SessionCookie:   10,
		CompressionPair: 1,
		VerifyCode:      [4]byte{7, 8, 9, 10},
		NegotiatedHybridCapabilities: SessionHybridCapabilities{
			HybridSupported: true,
			MaxFeedbackRate: 77,
			MaxStreams:      333,
		},
		HasHybridCapabilitySync: true,
		HasClientPolicySync:     false,
	}

	encoded := EncodeSessionAcceptPayload(payload)
	decoded, err := DecodeSessionAcceptPayload(encoded)
	if err != nil {
		t.Fatalf("DecodeSessionAcceptPayload returned error: %v", err)
	}

	if decoded.HasClientPolicySync {
		t.Fatal("did not expect policy sync block")
	}
	if !decoded.HasHybridCapabilitySync {
		t.Fatal("expected hybrid capability sync block")
	}
	if decoded.NegotiatedHybridCapabilities != payload.NegotiatedHybridCapabilities {
		t.Fatalf("unexpected decoded capabilities: got=%+v want=%+v", decoded.NegotiatedHybridCapabilities, payload.NegotiatedHybridCapabilities)
	}
}
