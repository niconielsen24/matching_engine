package engine

import (
	"slices"
	"testing"
)

//TestWorkedExample replays a sequence of commands through one engine, checking
//the exact events of each Apply and calling checkInvariants after every step:
//	limit orders resting on both sides without crossing
//	a second order at an existing price queues behind the first (FIFO)
//	one sell sweeping three makers across two levels, each filled at the maker's price
//	cancelling a partially filled resting order, reporting its remaining qty
//	cancelling the same order again is rejected (InvalidOrderID)
//	rejects: duplicate ID, zero qty, zero price, unset TIF, unknown command kind
//	a buy sweeping the last ask, then resting its leftover at its own price
//Final book: no asks, one bid at 200 with qty 6.
//Relies on checkInvariants and levelPrices from book_test.go.

func lim(seq uint64, id OrderID, s Side, p Price, q Quantity) Command {
	return Command{Seq: seq, Kind: New, ID: id, Side: s, Type: Limit, TIF: GTC, Price: p, Qty: q}
}

func TestWorkedExample(t *testing.T) {
	e := NewEngine()
	steps := []struct {
		cmd  Command
		want []Event
	}{
		{lim(1, 1, Buy, 99, 6), []Event{{Seq: 1, Kind: Accepted, Taker: 1}}},
		{lim(2, 2, Buy, 98, 8), []Event{{Seq: 2, Kind: Accepted, Taker: 2}}},
		{lim(3, 3, Sell, 101, 4), []Event{{Seq: 3, Kind: Accepted, Taker: 3}}},
		{lim(4, 4, Buy, 99, 2), []Event{{Seq: 4, Kind: Accepted, Taker: 4}}},
		{lim(5, 5, Sell, 98, 10), []Event{
			{Seq: 5, Kind: Accepted, Taker: 5},
			{Seq: 5, Kind: Fill, Maker: 1, Taker: 5, Price: 99, Qty: 6},
			{Seq: 5, Kind: Fill, Maker: 4, Taker: 5, Price: 99, Qty: 2},
			{Seq: 5, Kind: Fill, Maker: 2, Taker: 5, Price: 98, Qty: 2},
		}},
		{Command{Seq: 6, Kind: Cancel, ID: 2}, []Event{{Seq: 6, Kind: Cancelled, Maker: 2, Taker: 2, Price: 98, Qty: 6}}},
		{Command{Seq: 7, Kind: Cancel, ID: 2}, []Event{{Seq: 7, Kind: Rejected, Reason: InvalidOrderID}}},
		{lim(8, 3, Buy, 50, 1), []Event{{Seq: 8, Kind: Rejected, Reason: DuplicateOrderID}}},
		{lim(9, 9, Buy, 50, 0), []Event{{Seq: 9, Kind: Rejected, Reason: InvalidQty}}},
		{lim(10, 10, Buy, 0, 1), []Event{{Seq: 10, Kind: Rejected, Reason: InvalidPrice}}},
		{Command{Seq: 11, Kind: New, ID: 11, Side: Buy, Price: 50, Qty: 1}, []Event{{Seq: 11, Kind: Rejected, Reason: InvalidTIF}}},
		{Command{Seq: 12}, []Event{{Seq: 12, Kind: Rejected, Reason: UnknownCommand}}},
		{lim(13, 13, Buy, 200, 10), []Event{
			{Seq: 13, Kind: Accepted, Taker: 13},
			{Seq: 13, Kind: Fill, Maker: 3, Taker: 13, Price: 101, Qty: 4},
		}},
	}
	for i, s := range steps {
		got := e.Apply(s.cmd, nil)
		if !slices.Equal(got, s.want) {
			t.Fatalf("step %d:\n got  %+v\n want %+v", i+1, got, s.want)
		}
		checkInvariants(t, e.book)
	}
	if len(e.book.Asks) != 0 || len(e.book.Bids) != 1 || e.book.Bids[0].Price != 200 || e.book.Bids[0].Orders[0].Qty != 6 {
		t.Fatalf("final book wrong: bids=%v asks=%v", levelPrices(e.book.Bids), levelPrices(e.book.Asks))
	}
}
