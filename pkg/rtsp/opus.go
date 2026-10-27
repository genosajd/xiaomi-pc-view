package rtsp

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

// opusHeaders sends the Ogg Opus ID and comment headers (RFC 7845) as the first two RTP
// packets of the track. Android Media3 (RtpOpusReader, 1.5) needs them before the first
// audio frame ("ID Header missing"); other players don't, so only on request
// (?opus_headers=1).
func (c *Conn) opusHeaders(codec *core.Codec, next core.HandlerFunc) core.HandlerFunc {
	sent := false
	return func(packet *rtp.Packet) {
		if !sent && c.state != StateNone {
			sent = true
			channels := codec.Channels
			if channels == 0 {
				channels = 2
			}
			head := []byte("OpusHead\x01")
			head = append(head, channels)
			head = binary.LittleEndian.AppendUint16(head, 312) // pre-skip: the libopus default
			head = binary.LittleEndian.AppendUint32(head, codec.ClockRate)
			head = append(head, 0, 0, 0) // output gain, channel mapping family 0
			tags := []byte("OpusTags")
			tags = binary.LittleEndian.AppendUint32(tags, 6)
			tags = append(tags, "go2rtc"...)
			tags = binary.LittleEndian.AppendUint32(tags, 0) // no comments
			for i, payload := range [][]byte{head, tags} {
				next(&rtp.Packet{
					Header: rtp.Header{
						Marker:         true,
						SequenceNumber: packet.SequenceNumber - uint16(2-i),
						Timestamp:      packet.Timestamp,
						SSRC:           packet.SSRC,
					},
					Payload: payload,
				})
			}
		}
		next(packet)
	}
}
