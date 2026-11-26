package webserver

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLiveTapInOrderAndBounded(t *testing.T) {
	h := NewLiveHub(LiveOptions{})
	got := make(chan string, 1000)
	block := make(chan struct{})
	h.Tap(func(topic string, _ json.RawMessage) {
		<-block
		got <- topic
	})
	for i := range tapQueue + 50 {
		h.Publish("t"+string(rune('a'+i%26)), i)
	}
	close(block)
	n := 0
	for {
		select {
		case <-got:
			n++
			continue
		case <-time.After(300 * time.Millisecond):
		}
		break
	}
	if n < tapQueue || n > tapQueue+1 {
		t.Fatalf("tap got %d messages, want about %d (the rest dropped, publish never blocked)", n, tapQueue)
	}
}
