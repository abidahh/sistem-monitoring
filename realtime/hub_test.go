package realtime

import (
	"encoding/json"
	"testing"
	"time"
)

// collectClient mencatat seluruh event yang diterima klien uji.
type collectClient struct {
	*Client
	got []Event
}

func newCollectClient() *collectClient {
	return &collectClient{Client: &Client{send: make(chan []byte, 16)}}
}

func (cc *collectClient) drain() Event {
	select {
	case msg := <-cc.send:
		var ev Event
		json.Unmarshal(msg, &ev)
		cc.got = append(cc.got, ev)
		return ev
	default:
		return Event{}
	}
}

// TestHubFilter menguji fan-out: klien hanya menerima sensor yang diizinkan.
func TestHubFilter(t *testing.T) {
	h := NewHub()
	go h.Run()

	alice := newCollectClient()
	bob := newCollectClient()
	h.Register(alice.Client, []uint{1, 2})
	h.Register(bob.Client, []uint{2})

	h.Broadcast(Event{Type: "reading", SensorTypeID: 1, Value: 10})
	h.Broadcast(Event{Type: "reading", SensorTypeID: 2, Value: 20})

	time.Sleep(100 * time.Millisecond) // beri waktu Run() menyalurkan Event

	if got := alice.drain(); got.Type != "reading" || got.SensorTypeID != 1 {
		t.Fatalf("alice pertama = %+v, harap sensor 1", got)
	}
	if got := alice.drain(); got.SensorTypeID != 2 {
		t.Fatalf("alice kedua = %+v, harap sensor 2", got)
	}
	if got := bob.drain(); got.SensorTypeID != 2 {
		t.Fatalf("bob = %+v, harap sensor 2 (tidak boleh sensor 1)", got)
	}
}