package seat

import "testing"

func TestBrokerPublish(t *testing.T) {
	broker := NewBroker()
	events, unsubscribe := broker.Subscribe()
	defer unsubscribe()

	seatID := int64(42)
	broker.Publish(StreamEvent{Type: "seat_changed", SeatID: &seatID, State: StateMine})

	got := <-events
	if got.Type != "seat_changed" || got.SeatID == nil || *got.SeatID != 42 || got.State != StateMine {
		t.Fatalf("published event = %#v", got)
	}
}
