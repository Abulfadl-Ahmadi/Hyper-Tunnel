package vpnproto

import (
	"encoding/binary"
	"fmt"
)

const (
	SessionInitPayloadBaseSize          = 10
	SessionHybridCapabilityPayloadSize  = 5
)

type SessionHybridCapabilities struct {
	HybridSupported bool
	MaxFeedbackRate uint16
	MaxStreams      uint16
}

func EncodeSessionHybridCapabilities(capabilities SessionHybridCapabilities) [SessionHybridCapabilityPayloadSize]byte {
	var payload [SessionHybridCapabilityPayloadSize]byte
	if capabilities.HybridSupported {
		payload[0] = 0x01
	}
	binary.BigEndian.PutUint16(payload[1:3], capabilities.MaxFeedbackRate)
	binary.BigEndian.PutUint16(payload[3:5], capabilities.MaxStreams)
	return payload
}

func DecodeSessionHybridCapabilities(payload []byte) (SessionHybridCapabilities, error) {
	if len(payload) < SessionHybridCapabilityPayloadSize {
		return SessionHybridCapabilities{}, fmt.Errorf("session hybrid capability payload too short: got=%d want>=%d", len(payload), SessionHybridCapabilityPayloadSize)
	}

	return SessionHybridCapabilities{
		HybridSupported: payload[0]&0x01 != 0,
		MaxFeedbackRate: binary.BigEndian.Uint16(payload[1:3]),
		MaxStreams:      binary.BigEndian.Uint16(payload[3:5]),
	}, nil
}

func ParseSessionInitHybridCapabilities(payload []byte) (SessionHybridCapabilities, bool, error) {
	if len(payload) < SessionInitPayloadBaseSize {
		return SessionHybridCapabilities{}, false, fmt.Errorf("session init payload too short: got=%d want>=%d", len(payload), SessionInitPayloadBaseSize)
	}

	if len(payload) == SessionInitPayloadBaseSize {
		return SessionHybridCapabilities{}, false, nil
	}

	if len(payload) < SessionInitPayloadBaseSize+SessionHybridCapabilityPayloadSize {
		return SessionHybridCapabilities{}, false, fmt.Errorf("session init hybrid capability payload too short: got=%d want>=%d", len(payload), SessionInitPayloadBaseSize+SessionHybridCapabilityPayloadSize)
	}

	capabilities, err := DecodeSessionHybridCapabilities(payload[SessionInitPayloadBaseSize : SessionInitPayloadBaseSize+SessionHybridCapabilityPayloadSize])
	if err != nil {
		return SessionHybridCapabilities{}, false, err
	}
	return capabilities, true, nil
}

func AppendSessionInitHybridCapabilities(basePayload []byte, capabilities SessionHybridCapabilities) []byte {
	payload := make([]byte, len(basePayload)+SessionHybridCapabilityPayloadSize)
	copy(payload, basePayload)
	encoded := EncodeSessionHybridCapabilities(capabilities)
	copy(payload[len(basePayload):], encoded[:])
	return payload
}
