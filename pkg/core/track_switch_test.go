package core

import (
	"testing"

	"github.com/pion/rtp"
)

// A source switched in place (the on-air camera) starts its RTP clock anywhere: the viewer must
// see one continuous timeline.
func TestSenderKeepsOneTimelineAcrossASourceChange(t *testing.T) {
	codec := &Codec{Name: CodecH265, ClockRate: 90000}
	s := NewSender(nil, codec)
	var got []uint32
	s.Handler = func(p *Packet) { got = append(got, p.Timestamp) }
	feed := func(ts uint32) {
		s.Input(&rtp.Packet{Header: rtp.Header{Timestamp: ts}})
	}
	for i := uint32(0); i < 5; i++ {
		feed(1000 + i*4500) // camera A, 20 fps
	}
	shared := &rtp.Packet{Header: rtp.Header{Timestamp: 3_000_000_000}}
	s.Input(shared) // camera B: its clock is somewhere else
	feed(3_000_000_000 + 4500)
	feed(3_000_000_000 + 9000)
	s.Start()
	close(s.buf)
	s.Wait()
	if len(got) != 8 {
		t.Fatalf("got %d packets", len(got))
	}
	for i := 1; i < len(got); i++ {
		if d := int32(got[i] - got[i-1]); d <= 0 || d > 9000 {
			t.Fatalf("timeline breaks at %d: %v", i, got)
		}
	}
	if shared.Timestamp != 3_000_000_000 {
		t.Fatalf("the shared packet was changed: %d", shared.Timestamp)
	}
}
