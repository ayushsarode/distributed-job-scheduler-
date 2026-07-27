package twilio

// Inbound message types (messages we receive from Twilio)

// twilioInboundMessage represents the base structure for all incoming Twilio messages.
type twilioInboundMessage struct {
	Event          string `json:"event"`
	SequenceNumber string `json:"sequenceNumber,omitempty"`
	StreamSid      string `json:"streamSid,omitempty"`
}

// twilioConnectedMessage represents Twilio's connected message.
type twilioConnectedMessage struct {
	Event    string `json:"event"`
	Protocol string `json:"protocol"`
	Version  string `json:"version"`
}

// twilioStartMessage represents Twilio's start message.
type twilioStartMessage struct {
	Event          string          `json:"event"`
	SequenceNumber string          `json:"sequenceNumber"`
	Start          twilioStartData `json:"start"`
	StreamSid      string          `json:"streamSid"`
}

// twilioStartData contains the start message metadata.
type twilioStartData struct {
	StreamSid        string            `json:"streamSid"`
	AccountSid       string            `json:"accountSid"`
	CallSid          string            `json:"callSid"`
	Tracks           []string          `json:"tracks"`
	CustomParameters map[string]any    `json:"customParameters"`
	MediaFormat      twilioMediaFormat `json:"mediaFormat"`
}

// twilioMediaFormat describes the audio format.
type twilioMediaFormat struct {
	Encoding   string `json:"encoding"`   // always "audio/x-mulaw"
	SampleRate int    `json:"sampleRate"` // always 8000
	Channels   int    `json:"channels"`   // always 1
}

// twilioInboundMediaMessage represents incoming media messages from Twilio.
type twilioInboundMediaMessage struct {
	Event          string                 `json:"event"`
	SequenceNumber string                 `json:"sequenceNumber"`
	Media          twilioInboundMediaData `json:"media"`
	StreamSid      string                 `json:"streamSid"`
}

// twilioInboundMediaData contains the media payload from Twilio.
type twilioInboundMediaData struct {
	Track     string `json:"track"`     // "inbound" or "outbound"
	Chunk     string `json:"chunk"`     // chunk number
	Timestamp string `json:"timestamp"` // timestamp
	Payload   string `json:"payload"`   // base64 encoded mulaw audio
}

// twilioStopMessage represents Twilio's stop message.
type twilioStopMessage struct {
	Event          string         `json:"event"`
	SequenceNumber string         `json:"sequenceNumber"`
	Stop           twilioStopData `json:"stop"`
	StreamSid      string         `json:"streamSid"`
}

// twilioStopData contains stop message metadata.
type twilioStopData struct {
	AccountSid string `json:"accountSid"`
	CallSid    string `json:"callSid"`
}

// twilioDTMFMessage represents DTMF messages from Twilio.
type twilioDTMFMessage struct {
	Event          string         `json:"event"`
	SequenceNumber string         `json:"sequenceNumber"`
	DTMF           twilioDTMFData `json:"dtmf"`
	StreamSid      string         `json:"streamSid"`
}

// twilioDTMFData contains DTMF key press information.
type twilioDTMFData struct {
	Track string `json:"track"` // always "inbound_track"
	Digit string `json:"digit"` // the pressed digit
}

// twilioInboundMarkMessage represents mark messages from Twilio.
type twilioInboundMarkMessage struct {
	Event          string                `json:"event"`
	SequenceNumber string                `json:"sequenceNumber"`
	Mark           twilioInboundMarkData `json:"mark"`
	StreamSid      string                `json:"streamSid"`
}

// twilioInboundMarkData contains mark metadata from Twilio.
type twilioInboundMarkData struct {
	Name string `json:"name"`
}

// Outbound message types (messages we send to Twilio)

// twilioOutboundMediaMessage represents a media message we send to Twilio.
type twilioOutboundMediaMessage struct {
	Event     string                     `json:"event"`
	StreamSid string                     `json:"streamSid"`
	Media     twilioOutboundMediaPayload `json:"media"`
}

// twilioOutboundMediaPayload represents the media payload we send to Twilio.
type twilioOutboundMediaPayload struct {
	Payload string `json:"payload"` // Raw mulaw/8000 audio encoded in base64
}

// twilioOutboundMarkMessage represents a mark message we send to Twilio.
type twilioOutboundMarkMessage struct {
	Event     string                 `json:"event"`
	StreamSid string                 `json:"streamSid"`
	Mark      twilioOutboundMarkData `json:"mark"`
}

// twilioOutboundMarkData represents the mark data we send to Twilio.
type twilioOutboundMarkData struct {
	Name string `json:"name"`
}

// twilioClearMessage represents a clear message we send to Twilio.
type twilioClearMessage struct {
	Event     string `json:"event"`
	StreamSid string `json:"streamSid"`
}
