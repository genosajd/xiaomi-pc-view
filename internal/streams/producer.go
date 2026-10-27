package streams

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type state byte

const (
	stateNone state = iota
	stateMedias
	stateTracks
	stateStart
	stateExternal
	stateInternal
)

type Producer struct {
	core.Listener

	url      string
	template string

	conn      core.Producer
	receivers []*core.Receiver
	senders   []*core.Receiver

	state    state
	mu       sync.Mutex
	workerID int

	// dialMu: one dial at a time (a Xiaomi camera takes one session). The network part of a
	// dial runs without mu, so SetSource, stop and the info calls never wait for a camera
	// that is slow to answer (the app's UI froze for the length of a dial). gen counts the
	// stops and source changes: a dial that finishes after one is thrown away.
	dialMu sync.Mutex
	gen    int
}

const SourceTemplate = "{input}"

func NewProducer(source string) *Producer {
	if strings.Contains(source, SourceTemplate) {
		return &Producer{template: source}
	}

	return &Producer{url: source}
}

// SetSource changes the source URL. A running connection with another URL is stopped, so
// the next consumer dials the new source: the consumer of a stopped player is removed
// asynchronously and a new one could otherwise join the old connection (e.g. history
// showed live video instead of the SD card recording).
func (p *Producer) SetSource(s string) {
	if p.template != "" {
		s = strings.Replace(p.template, SourceTemplate, s, 1)
	}
	p.mu.Lock()
	changed := p.url != s
	p.url = s
	if changed {
		p.gen++
	}
	running := changed && p.state == stateStart && p.conn != nil
	var id int
	if running {
		p.workerID++ // (the old connection's worker then ends without reconnecting)
		id = p.workerID
	}
	p.mu.Unlock()
	if running {
		// A running stream changes its source in place: reconnect dials the new url, moves the
		// consumers' tracks onto it (receiver.Replace) and only then stops the old connection, so
		// a viewer keeps its session - OBS on the on-air stream switches camera without a drop,
		// and sees the old camera until the new source answers. (stop() here closed the tracks
		// with the viewers on them; stopping the old connection first did too.)
		go p.reconnect(id, 0)
		return
	}
	if changed {
		p.stop()
	}
}

func (p *Producer) Dial() error {
	p.dialMu.Lock()
	defer p.dialMu.Unlock()

	p.mu.Lock()
	if p.state != stateNone {
		p.mu.Unlock()
		return nil
	}
	url, gen := p.url, p.gen
	p.mu.Unlock()

	conn, err := GetProducer(url) // the network part: without mu
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.gen != gen || p.state != stateNone {
		_ = conn.Stop() // stopped or another source meanwhile
		if p.gen != gen {
			return errors.New("streams: source changed while dialing")
		}
		return nil
	}
	p.conn = conn
	p.state = stateMedias
	return nil
}

func (p *Producer) GetMedias() []*core.Media {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil {
		return nil
	}

	return p.conn.GetMedias()
}

func (p *Producer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		return nil, errors.New("get track from none state")
	}

	for _, track := range p.receivers {
		if track.Codec == codec {
			return track, nil
		}
	}

	track, err := p.conn.GetTrack(media, codec)
	if err != nil {
		return nil, err
	}

	p.receivers = append(p.receivers, track)

	if p.state == stateMedias {
		p.state = stateTracks
	}

	return track, nil
}

func (p *Producer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == stateNone {
		return errors.New("add track from none state")
	}

	if err := p.conn.(core.Consumer).AddTrack(media, codec, track); err != nil {
		return err
	}

	p.senders = append(p.senders, track)

	if p.state == stateMedias {
		p.state = stateTracks
	}

	return nil
}

func (p *Producer) MarshalJSON() ([]byte, error) {
	if conn := p.conn; conn != nil {
		return json.Marshal(conn)
	}
	info := map[string]string{"url": p.url}
	return json.Marshal(info)
}

// internals

func (p *Producer) start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != stateTracks {
		return
	}

	log.Debug().Msgf("[streams] start producer url=%s", p.url)

	p.state = stateStart
	p.workerID++

	go p.worker(p.conn, p.workerID)
}

func (p *Producer) worker(conn core.Producer, workerID int) {
	if err := conn.Start(); err != nil {
		p.mu.Lock()
		closed := p.workerID != workerID
		p.mu.Unlock()

		if closed {
			return
		}

		log.Warn().Err(err).Str("url", p.url).Caller().Send()
	}

	p.reconnect(workerID, 0)
}

func (p *Producer) reconnect(workerID, retry int) {
	p.mu.Lock()
	if p.workerID != workerID {
		log.Trace().Msgf("[streams] stop reconnect url=%s", p.url)
		p.mu.Unlock()
		return
	}
	url, gen := p.url, p.gen
	p.mu.Unlock()

	log.Debug().Msgf("[streams] retry=%d to url=%s", retry, url)

	p.dialMu.Lock()
	conn, err := GetProducer(url) // the network part: without mu (see Producer.dialMu)
	p.dialMu.Unlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.workerID != workerID || p.gen != gen {
		if conn != nil {
			_ = conn.Stop() // stopped or another source meanwhile
		}
		return
	}
	if err != nil {
		log.Debug().Msgf("[streams] producer=%s", err)

		timeout := time.Minute
		if retry < 5 {
			timeout = time.Second
		} else if retry < 10 {
			timeout = time.Second * 5
		} else if retry < 20 {
			timeout = time.Second * 10
		}

		time.AfterFunc(timeout, func() {
			p.reconnect(workerID, retry+1)
		})
		return
	}

	for _, media := range conn.GetMedias() {
		switch media.Direction {
		case core.DirectionRecvonly:
			for i, receiver := range p.receivers {
				codec := media.MatchCodec(receiver.Codec)
				if codec == nil {
					continue
				}

				track, err := conn.GetTrack(media, codec)
				if err != nil {
					continue
				}

				receiver.Replace(track)
				p.receivers[i] = track
				break
			}

		case core.DirectionSendonly:
			for _, sender := range p.senders {
				codec := media.MatchCodec(sender.Codec)
				if codec == nil {
					continue
				}

				_ = conn.(core.Consumer).AddTrack(media, codec, sender)
			}
		}
	}

	// stop previous connection after moving tracks (fix ghost exec/ffmpeg)
	_ = p.conn.Stop()
	// swap connections
	p.conn = conn

	go p.worker(conn, workerID)
}

func (p *Producer) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.gen++ // a dial running now is thrown away when it finishes

	switch p.state {
	case stateExternal:
		log.Trace().Msgf("[streams] skip stop external producer")
		return
	case stateNone:
		log.Trace().Msgf("[streams] skip stop none producer")
		return
	case stateStart:
		p.workerID++
	}

	log.Debug().Msgf("[streams] stop producer url=%s", p.url)

	if p.conn != nil {
		_ = p.conn.Stop()
		p.conn = nil
	}

	p.state = stateNone
	p.receivers = nil
	p.senders = nil
}
