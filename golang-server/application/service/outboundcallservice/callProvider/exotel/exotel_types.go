package exotel

// Inbound message types (messages we receive from Exotel)

// exotelInboundMessage represents the base structure for all incoming Exotel messages.
type exotelInboundMessage struct {
	Event          string `json:"event"`
	SequenceNumber string `json:"sequence_number,omitempty"`
	StreamSid      string `json:"stream_sid,omitempty"`
}

// exotelConnectedMessage represents Exotel's connected message.
type exotelConnectedMessage struct {
	Event string `json:"event"`
}

// exotelStartMessage represents Exotel's start message.
type exotelStartMessage struct {
	Event          string          `json:"event"`
	SequenceNumber string          `json:"sequence_number"`
	StreamSid      string          `json:"stream_sid"`
	Start          exotelStartData `json:"start"`
}

// exotelStartData contains metadata about the start event.
type exotelStartData struct {
	StreamSid        string            `json:"stream_sid"`
	CallSid          string            `json:"call_sid"`
	AccountSid       string            `json:"account_sid"`
	From             string            `json:"from"`
	To               string            `json:"to"`
	CustomParameters map[string]any    `json:"custom_parameters"`
	MediaFormat      exotelMediaFormat `json:"media_format"`
}

// exotelMediaFormat describes the audio format.
type exotelMediaFormat struct {
	Encoding   string `json:"encoding"`    // e.g. "audio/raw"
	SampleRate string `json:"sample_rate"` // 8000
	BitRate    string `json:"bit_rate"`    // optional; present in some events
}

// exotelInboundMediaMessage represents media chunks from Exotel.
type exotelInboundMediaMessage struct {
	Event          string                 `json:"event"`
	SequenceNumber string                 `json:"sequence_number"`
	StreamSid      string                 `json:"stream_sid"`
	Media          exotelInboundMediaData `json:"media"`
}

// exotelInboundMediaData contains media payload info.
type exotelInboundMediaData struct {
	Chunk     string `json:"chunk"`
	Timestamp string `json:"timestamp"`
	Payload   string `json:"payload"` // base64-encoded raw/slin audio
}

// exotelDTMFMessage represents DTMF tone messages.
type exotelDTMFMessage struct {
	Event          string         `json:"event"`
	SequenceNumber string         `json:"sequence_number"`
	StreamSid      string         `json:"stream_sid"`
	DTMF           exotelDTMFData `json:"dtmf"`
}

// exotelDTMFData holds DTMF digit details.
type exotelDTMFData struct {
	Digit    string `json:"digit"`
	Duration string `json:"duration"`
}

// exotelStopMessage represents the stop event.
type exotelStopMessage struct {
	Event          string         `json:"event"`
	SequenceNumber string         `json:"sequence_number"`
	StreamSid      string         `json:"stream_sid"`
	Stop           exotelStopData `json:"stop"`
}

// exotelStopData contains stop metadata.
type exotelStopData struct {
	CallSid    string `json:"call_sid"`
	AccountSid string `json:"account_sid"`
	Reason     string `json:"reason"`
}

// exotelInboundMarkMessage represents a mark event from Exotel.
type exotelInboundMarkMessage struct {
	Event          string                `json:"event"`
	SequenceNumber string                `json:"sequence_number"`
	StreamSid      string                `json:"stream_sid"`
	Mark           exotelInboundMarkData `json:"mark"`
}

// exotelInboundMarkData contains mark metadata.
type exotelInboundMarkData struct {
	Name string `json:"name"`
}

// Outbound message types (messages we send to Exotel)

// exotelOutboundMediaMessage represents outbound audio packets.
type exotelOutboundMediaMessage struct {
	Event          string                     `json:"event"`
	SequenceNumber string                     `json:"sequence_number"`
	StreamSid      string                     `json:"stream_sid"`
	Media          exotelOutboundMediaPayload `json:"media"`
}

// exotelOutboundMediaPayload represents audio data sent to Exotel.
type exotelOutboundMediaPayload struct {
	Chunk     string `json:"chunk"`
	Timestamp string `json:"timestamp"`
	Payload   string `json:"payload"` // base64 encoded slin audio
}

// exotelOutboundMarkMessage represents a mark message sent to Exotel.
type exotelOutboundMarkMessage struct {
	Event          string                 `json:"event"`
	SequenceNumber string                 `json:"sequence_number"`
	StreamSid      string                 `json:"stream_sid"`
	Mark           exotelOutboundMarkData `json:"mark"`
}

// exotelOutboundMarkData contains outbound mark info.
type exotelOutboundMarkData struct {
	Name string `json:"name"`
}

// exotelClearMessage represents a clear message we send to Exotel.
type exotelClearMessage struct {
	Event     string `json:"event"`
	StreamSid string `json:"stream_sid"`
}
