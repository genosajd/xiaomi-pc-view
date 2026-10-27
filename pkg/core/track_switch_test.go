package core

import (
	"testing"

	"github.com/pion/rtp"
)

// A sender moved to another source (MoveNode: the on-air camera switched in place) starts
// getting packets with another RTP clock: the viewer must see one continuous timeline. A gap
// inside one source stays as it is.
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
	feed(1000 + 4*4500 + 10*90000) // a 10 s stall of camera A: kept
	old, now := &Receiver{}, &Receiver{}
	old.Node.AppendChild(&s.Node)
	MoveNode(&now.Node, &old.Node) // the tracks moved to camera B
	shared := &rtp.Packet{Header: rtp.Header{Timestamp: 3_000_000_000}}
	s.Input(shared) // camera B: its clock is somewhere else
	feed(3_000_000_000 + 4500)
	feed(3_000_000_000 + 9000)
	s.Start()
	close(s.buf)
	s.Wait()
	if len(got) != 9 {
		t.Fatalf("got %d packets", len(got))
	}
	if d := int32(got[5] - got[4]); d != 10*90000 {
		t.Fatalf("the stall was changed: %d", d)
	}
	for i := 6; i < len(got); i++ {
		if d := int32(got[i] - got[i-1]); d <= 0 || d > 9000 {
			t.Fatalf("timeline breaks at %d: %v", i, got)
		}
	}
	if shared.Timestamp != 3_000_000_000 {
		t.Fatalf("the shared packet was changed: %d", shared.Timestamp)
	}
}
