package recorder

import (
	"testing"
	"time"
)

// TestDrainReturnsWhenIdle verifies that shutdown doesn't wait out the full window when nothing
// is in flight.
func TestDrainReturnsWhenIdle(t *testing.T) {
	consumer := &Consumer{}
	done := make(chan struct{})
	go func() {
		consumer.Drain(30 * time.Second)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Drain blocked with no deliveries in flight")
	}
}

// TestDrainWaitsForInFlightDeliveries verifies that shutdown holds the connections open until a
// delivery that is mid-record finishes, since Recorder.Record publishes after it commits.
func TestDrainWaitsForInFlightDeliveries(t *testing.T) {
	consumer := &Consumer{}
	consumer.inFlight.Add(1)

	done := make(chan struct{})
	go func() {
		consumer.Drain(5 * time.Second)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Drain returned while a delivery was still in flight")
	case <-time.After(300 * time.Millisecond):
	}

	consumer.inFlight.Add(-1)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Drain did not return after the delivery finished")
	}
}
