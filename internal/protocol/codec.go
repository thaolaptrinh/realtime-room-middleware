package protocol

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// encodeRaw marshals an envelope without validation, used only in tests.
func encodeRaw(env *Envelope) ([]byte, error) {
	return msgpack.Marshal(env)
}

// EncodeEnvelope serializes an Envelope to MessagePack bytes.
// The caller should set Version, Type, Seq, Tick, and Body before calling.
func EncodeEnvelope(env *Envelope) ([]byte, error) {
	if err := ValidateVersion(env.Version); err != nil {
		return nil, err
	}
	if err := ValidatePayloadSize(env.Body); err != nil {
		return nil, err
	}
	data, err := msgpack.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("encode envelope: %w", err)
	}
	if err := ValidatePacketSize(data); err != nil {
		return nil, err
	}
	return data, nil
}

// DecodeEnvelope deserializes MessagePack bytes into an Envelope.
// It validates version and payload size before returning.
func DecodeEnvelope(data []byte) (*Envelope, error) {
	if err := ValidatePacketSize(data); err != nil {
		return nil, err
	}
	var env Envelope
	if err := msgpack.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	if err := ValidateVersion(env.Version); err != nil {
		return nil, err
	}
	if err := ValidatePayloadSize(env.Body); err != nil {
		return nil, err
	}
	return &env, nil
}

// EncodeMessage serializes a message struct to MessagePack bytes
// for use as an Envelope body.
func EncodeMessage(msg interface{}) ([]byte, error) {
	data, err := msgpack.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encode message: %w", err)
	}
	if err := ValidatePayloadSize(data); err != nil {
		return nil, err
	}
	return data, nil
}

// DecodeMessage deserializes MessagePack bytes from an Envelope body
// into the provided message struct pointer.
func DecodeMessage(body []byte, msg interface{}) error {
	if err := msgpack.Unmarshal(body, msg); err != nil {
		return fmt.Errorf("decode message: %w", err)
	}
	return nil
}

// BuildEnvelope creates a fully populated Envelope with the body encoded from msg.
func BuildEnvelope(version uint16, msgType MessageType, seq uint32, tick uint32, msg interface{}) (*Envelope, error) {
	body, err := EncodeMessage(msg)
	if err != nil {
		return nil, err
	}
	return &Envelope{
		Version: version,
		Type:    msgType,
		Seq:     seq,
		Tick:    tick,
		Body:    body,
	}, nil
}

// EncodeAndWrap is a convenience function that builds and encodes an envelope
// in one step. Returns the final wire bytes.
func EncodeAndWrap(version uint16, msgType MessageType, seq uint32, tick uint32, msg interface{}) ([]byte, error) {
	env, err := BuildEnvelope(version, msgType, seq, tick, msg)
	if err != nil {
		return nil, err
	}
	return EncodeEnvelope(env)
}

// DecodeAndUnwrap is a convenience function that decodes an envelope and
// then decodes its body into the provided message struct.
func DecodeAndUnwrap(data []byte, msg interface{}) (*Envelope, error) {
	env, err := DecodeEnvelope(data)
	if err != nil {
		return nil, err
	}
	if err := DecodeMessage(env.Body, msg); err != nil {
		return nil, err
	}
	return env, nil
}
