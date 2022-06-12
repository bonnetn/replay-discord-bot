package circular

import (
	"github.com/bwmarrin/discordgo"
	"sync"
	"time"
)

const SIZE = 30 * 60 / 0.02 // 30 minutes of 20ms segments.

// Buffer contains audio packet.
// Zero value is safe to use and is equivalent to an empty buffer.
type Buffer struct {
	sync.RWMutex
	buffer       [SIZE]AudioPacket
	size         int
	nextPosition int
}

type Iterator struct {
	buffer   *Buffer
	position int
	count    int
}

type AudioPacket struct {
	Time     time.Time
	SSRC     uint32
	PCMIndex uint32
	Opus     []byte
}

func (b *Buffer) Add(t time.Time, pkt discordgo.Packet) {
	b.Lock()
	defer b.Unlock()

	b.buffer[b.nextPosition] = AudioPacket{
		Time:     t,
		SSRC:     pkt.SSRC,
		PCMIndex: pkt.Timestamp,
		Opus:     pkt.Opus,
	}

	if b.size < SIZE {
		b.size++
	}

	b.nextPosition++
	if b.nextPosition >= SIZE {
		b.nextPosition = 0
	}
}

func (b *Buffer) WithIterator(cb func(iterator *Iterator) error) error {
	b.RLock()
	defer b.RUnlock()

	position := b.nextPosition - b.size
	if position < 0 {
		position += SIZE
	}

	return cb(&Iterator{
		buffer:   b,
		position: position,
		count:    b.size,
	})
}

func (b *Buffer) Reset() {
	b.Lock()
	defer b.Unlock()

	b.size = 0
	b.nextPosition = 0
}

func (i *Iterator) HasNext() bool {
	return i.count > 0
}

func (i *Iterator) Next() *AudioPacket {
	if !i.HasNext() {
		panic("iterator is exhausted")
	}

	value := &i.buffer.buffer[i.position]

	i.position++
	if i.position >= SIZE {
		i.position = 0
	}

	i.count--
	return value
}
