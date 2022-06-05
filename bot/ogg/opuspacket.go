package ogg

import "time"

type OpusPacket struct {
	Opus      []byte
	Timestamp uint32
	SSRC      uint32
	RealTime  time.Time
}
