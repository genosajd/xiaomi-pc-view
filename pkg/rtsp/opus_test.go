package rtsp

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestOpusHeaders(t *testing.T) {
	c := &Conn{}
	var got []*rtp.Packet
	h := c.opusHeaders(&core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}, func(p *rtp.Packet) {
		got = append(got, p)
	})

	h(&rtp.Packet{Header: rtp.Header{SequenceNumber: 100, Timestamp: 5}, Payload: []byte{1}}) // before SETUP
	require.Len(t, got, 1)

	c.state = StatePlay
	h(&rtp.Packet{Header: rtp.Header{SequenceNumber: 101, Timestamp: 7}, Payload: []byte{2}})
	h(&rtp.Packet{Header: rtp.Header{SequenceNumber: 102, Timestamp: 9}, Payload: []byte{3}})
	require.Len(t, got, 5)

	head := got[1].Payload
	require.Equal(t, "OpusHead", string(head[:8]))
	require.Len(t, head, 19)
	require.Equal(t, byte(2), head[9])                      // channels
	require.Equal(t, []byte{0x80, 0xbb, 0, 0}, head[12:16]) // 48000 LE
	require.Equal(t, "OpusTags", string(got[2].Payload[:8]))
	require.Equal(t, []uint16{99, 100, 101}, []uint16{got[1].SequenceNumber, got[2].SequenceNumber, got[3].SequenceNumber})
	require.Equal(t, uint32(7), got[1].Timestamp)
}
