package telemetry

import "testing"

func TestChangeBusCoalescesUpdatesAndUnsubscribes(t *testing.T) {
	bus := NewChangeBus()
	updates, unsubscribe := bus.Subscribe()
	bus.Publish(Update{Signal: SignalLogs, ReceivedAtNS: 10})
	bus.Publish(Update{Signal: SignalTraces, ReceivedAtNS: 20})
	update := <-updates
	if update.Signal != SignalTraces || update.ReceivedAtNS != 20 {
		t.Fatalf("coalesced update = %+v, want latest trace update", update)
	}
	unsubscribe()
	bus.Publish(Update{Signal: SignalMetrics, ReceivedAtNS: 30})
	if _, open := <-updates; open {
		t.Fatal("unsubscribed channel remained open")
	}
}
