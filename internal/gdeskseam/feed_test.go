package gdeskseam

import (
	"sync"
	"testing"
)

func TestPublishReachesEveryConnection(t *testing.T) {
	feed := NewFeed()
	first := feed.Subscribe()
	second := feed.Subscribe()
	if feed.Readers() != 2 {
		t.Fatalf("readers = %d", feed.Readers())
	}

	feed.Publish(Event{Envelope: Envelope{Seq: 1}})

	for name, reader := range map[string]*Reader{"first": first, "second": second} {
		if got := reader.Drain(); len(got) != 1 || got[0].Seq != 1 {
			t.Fatalf("%s drained %+v", name, seqsOf(got))
		}
	}
}

func TestDrainIsEmptyUntilSomethingIsPublished(t *testing.T) {
	reader := NewFeed().Subscribe()
	if got := reader.Drain(); got != nil {
		t.Fatalf("drained %+v, want nothing", got)
	}
}

func TestDrainTakesEverythingOldestFirst(t *testing.T) {
	feed := NewFeed()
	reader := feed.Subscribe()
	for seq := int64(1); seq <= 3; seq++ {
		feed.Publish(Event{Envelope: Envelope{Seq: seq}})
	}

	got := seqsOf(reader.Drain())
	want := []int64{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("drained %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("drained %v, want %v", got, want)
		}
	}
	if again := reader.Drain(); again != nil {
		t.Fatalf("drained %+v twice", again)
	}
}

func TestReadyWakesAWritePumpOnce(t *testing.T) {
	feed := NewFeed()
	reader := feed.Subscribe()
	feed.Publish(Event{Envelope: Envelope{Seq: 1}})
	feed.Publish(Event{Envelope: Envelope{Seq: 2}})

	<-reader.Ready()
	select {
	case <-reader.Ready():
		t.Fatal("a second signal for events one drain already takes")
	default:
	}
}

func TestOfferDropsWhatAConnectionCannotKeepUpWith(t *testing.T) {
	feed := NewFeed()
	reader := feed.Subscribe()
	for seq := int64(1); seq <= MaxPending+5; seq++ {
		feed.Publish(Event{Envelope: Envelope{Seq: seq}})
	}

	got := reader.Drain()
	if len(got) != MaxPending {
		t.Fatalf("queued %d, want the ceiling %d", len(got), MaxPending)
	}
	if got[0].Seq != 1 {
		t.Fatalf("oldest = %d, want the queue kept contiguous from the front", got[0].Seq)
	}
	if last := got[len(got)-1].Seq; last != MaxPending {
		t.Fatalf("newest = %d, want the gap at the end where a resync starts", last)
	}
}

func TestCloseUnsubscribesAndWakesTheWritePump(t *testing.T) {
	feed := NewFeed()
	reader := feed.Subscribe()
	reader.Close()

	if feed.Readers() != 0 {
		t.Fatalf("readers = %d, want the feed to stop filling a queue nobody reads", feed.Readers())
	}
	select {
	case <-reader.Ready():
	default:
		t.Fatal("a closed reader must wake whoever waits on it")
	}
}

func TestCloseTwiceIsNotAPanic(t *testing.T) {
	reader := NewFeed().Subscribe()
	reader.Close()
	reader.Close()
}

func TestPublishToAClosedReaderIsDropped(t *testing.T) {
	feed := NewFeed()
	reader := feed.Subscribe()
	reader.Close()

	reader.offer(Event{Envelope: Envelope{Seq: 1}})
	if got := reader.Drain(); got != nil {
		t.Fatalf("drained %+v after close", got)
	}
}

func TestPublishAndCloseDoNotRaceEachOtherIntoAClosedChannel(t *testing.T) {
	feed := NewFeed()
	readers := make([]*Reader, 32)
	for i := range readers {
		readers[i] = feed.Subscribe()
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		for seq := int64(1); seq <= 200; seq++ {
			feed.Publish(Event{Envelope: Envelope{Seq: seq}})
		}
	})
	for _, reader := range readers {
		wg.Go(reader.Close)
	}
	wg.Wait()
}
