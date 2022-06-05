package circular

import (
	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func samplePacket(i int) discordgo.Packet {
	return discordgo.Packet{
		SSRC:      uint32(i),
		Timestamp: uint32(i),
	}
}

func sampleTime(i int) time.Time {
	return time.Unix(int64(i), 0)
}

func TestBuffer(t *testing.T) {
	tests := []struct {
		name             string
		elementsInserted int
		expectedCount    int
		oldestElement    int
	}{
		{
			name:             "10 elements",
			elementsInserted: 10,
			expectedCount:    10,
			oldestElement:    0,
		},
		{
			name:             "MAX_SIZE elements",
			elementsInserted: SIZE,
			expectedCount:    SIZE,
			oldestElement:    0,
		},
		{
			name:             "1.5 * MAX_SIZE elements",
			elementsInserted: 1.5 * SIZE,
			expectedCount:    SIZE,
			oldestElement:    SIZE / 2,
		},
		{
			name:             "2 * MAX_SIZE elements",
			elementsInserted: 2 * SIZE,
			expectedCount:    SIZE,
			oldestElement:    SIZE,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := Buffer{}

			for i := 0; i < tt.elementsInserted; i++ {
				b.Add(sampleTime(i), samplePacket(i))
			}

			var counter int
			got := b.WithIterator(func(iterator Iterator) error {
				for iterator.HasNext() {
					elem := iterator.Next()
					require.Equal(t, sampleTime(counter+tt.oldestElement), elem.Time)
					require.Equal(t, samplePacket(counter+tt.oldestElement), elem.Audio)
					counter += 1
				}
				return nil
			})
			assert.Equal(t, tt.expectedCount, counter)
			assert.Nil(t, got)
		})
	}
}
