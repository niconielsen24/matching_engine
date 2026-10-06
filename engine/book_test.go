package engine

import (
	"fmt"
	"slices"
	"testing"
)

//checkInvariants(t, b) should check:
//	bids are strictly descending and asks strictly ascending
//	no level is empty and every Qty is greater than 0
//	best bid is below best ask
//	every order sits under its own ID in Idx, and the order count matches len(b.Idx)
//Short tests, calling checkInvariants after each step:
//	insert at a new level
//	insert at an existing level, and check FIFO order
//	pop until a level empties, and check that the next level becomes best
//	remove from the middle of a level
//	remove the last order of a level
//	remove an unknown ID, which should return false and leave the book unchanged

func checkInvariants(t *testing.T, b *Book) {
	t.Helper()

	count := 0
	checkSide := func(name string, side Side, levels []*Level) {
		for i, level := range levels {
			if i > 0 {
				prev := levels[i-1].Price
				if side == Buy && level.Price >= prev {
					t.Fatalf("%s not strictly descending: %d after %d", name, level.Price, prev)
				}
				if side == Sell && level.Price <= prev {
					t.Fatalf("%s not strictly ascending: %d after %d", name, level.Price, prev)
				}
			}
			if len(level.Orders) == 0 {
				t.Fatalf("%s level %d is empty", name, level.Price)
			}
			for _, o := range level.Orders {
				if o.Qty <= 0 {
					t.Fatalf("order %d has qty %d", o.OrderID, o.Qty)
				}
				if o.Side != side || o.Price != level.Price {
					t.Fatalf("order %d (side %d, price %d) sits in %s level %d", o.OrderID, o.Side, o.Price, name, level.Price)
				}
				if got, ok := b.Idx[o.OrderID]; !ok || got != o {
					t.Fatalf("order %d not indexed under its own ID", o.OrderID)
				}
				count++
			}
		}
	}
	checkSide("bids", Buy, b.Bids)
	checkSide("asks", Sell, b.Asks)

	if bid, ask := b.Best(Buy), b.Best(Sell); bid != nil && ask != nil && bid.Price >= ask.Price {
		t.Fatalf("book crossed: best bid %d >= best ask %d", bid.Price, ask.Price)
	}
	if count != len(b.Idx) {
		t.Fatalf("book holds %d orders but Idx has %d", count, len(b.Idx))
	}
}

func newOrder(id OrderID, side Side, price Price, qty Quantity) *Order {
	return &Order{OrderID: id, Side: side, Type: Limit, Price: price, Qty: qty}
}

// levelIDs returns the order IDs resting at the given price, in queue order.
func levelIDs(t *testing.T, b *Book, side Side, price Price) []OrderID {
	t.Helper()
	i, found := b.findLevel(side, price)
	if !found {
		return nil
	}
	var ids []OrderID
	for _, o := range (*b.levels(side))[i].Orders {
		ids = append(ids, o.OrderID)
	}
	return ids
}

func levelPrices(levels []*Level) []Price {
	var prices []Price
	for _, l := range levels {
		prices = append(prices, l.Price)
	}
	return prices
}

func TestInsertNewLevel(t *testing.T) {
	b := NewBook()

	b.Insert(newOrder(1, Buy, 100, 10))
	checkInvariants(t, b)
	b.Insert(newOrder(2, Buy, 102, 10))
	checkInvariants(t, b)
	b.Insert(newOrder(3, Buy, 101, 10))
	checkInvariants(t, b)
	b.Insert(newOrder(4, Sell, 105, 10))
	checkInvariants(t, b)
	b.Insert(newOrder(5, Sell, 103, 10))
	checkInvariants(t, b)
	b.Insert(newOrder(6, Sell, 104, 10))
	checkInvariants(t, b)

	if got, want := levelPrices(b.Bids), []Price{102, 101, 100}; !slices.Equal(got, want) {
		t.Errorf("bid levels = %v, want %v", got, want)
	}
	if got, want := levelPrices(b.Asks), []Price{103, 104, 105}; !slices.Equal(got, want) {
		t.Errorf("ask levels = %v, want %v", got, want)
	}
	if got := b.Best(Buy).Price; got != 102 {
		t.Errorf("best bid = %d, want 102", got)
	}
	if got := b.Best(Sell).Price; got != 103 {
		t.Errorf("best ask = %d, want 103", got)
	}
}

func TestInsertExistingLevelFIFO(t *testing.T) {
	b := NewBook()

	b.Insert(newOrder(1, Sell, 100, 5))
	checkInvariants(t, b)
	b.Insert(newOrder(2, Sell, 100, 7))
	checkInvariants(t, b)
	b.Insert(newOrder(3, Sell, 100, 9))
	checkInvariants(t, b)

	if len(b.Asks) != 1 {
		t.Fatalf("ask levels = %d, want 1", len(b.Asks))
	}
	if got, want := levelIDs(t, b, Sell, 100), []OrderID{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("queue at 100 = %v, want %v", got, want)
	}

	for _, want := range []OrderID{1, 2, 3} {
		o, ok := b.PopBest(Sell)
		if !ok || o.OrderID != want {
			t.Fatalf("PopBest = %v, %v; want order %d", o, ok, want)
		}
		checkInvariants(t, b)
	}
}

func TestPopBestEmptiesLevel(t *testing.T) {
	b := NewBook()

	b.Insert(newOrder(1, Buy, 101, 10))
	b.Insert(newOrder(2, Buy, 101, 10))
	b.Insert(newOrder(3, Buy, 100, 10))
	checkInvariants(t, b)

	for _, want := range []OrderID{1, 2} {
		o, ok := b.PopBest(Buy)
		if !ok || o.OrderID != want {
			t.Fatalf("PopBest = %v, %v; want order %d", o, ok, want)
		}
		checkInvariants(t, b)
		if _, ok := b.Get(want); ok {
			t.Errorf("order %d still indexed after pop", want)
		}
	}

	best := b.Best(Buy)
	if best == nil || best.Price != 100 {
		t.Fatalf("best bid = %v, want level 100", best)
	}

	o, ok := b.PopBest(Buy)
	if !ok || o.OrderID != 3 {
		t.Fatalf("PopBest = %v, %v; want order 3", o, ok)
	}
	checkInvariants(t, b)

	if best := b.Best(Buy); best != nil {
		t.Errorf("best bid = %v, want nil on empty side", best)
	}
	if o, ok := b.PopBest(Buy); ok {
		t.Errorf("PopBest on empty side = %v, want false", o)
	}
}

func TestRemoveMiddleOfLevel(t *testing.T) {
	b := NewBook()

	b.Insert(newOrder(1, Sell, 100, 10))
	b.Insert(newOrder(2, Sell, 100, 10))
	b.Insert(newOrder(3, Sell, 100, 10))
	checkInvariants(t, b)

	o, ok := b.Remove(2)
	if !ok || o.OrderID != 2 {
		t.Fatalf("Remove(2) = %v, %v; want order 2", o, ok)
	}
	checkInvariants(t, b)

	if got, want := levelIDs(t, b, Sell, 100), []OrderID{1, 3}; !slices.Equal(got, want) {
		t.Errorf("queue at 100 = %v, want %v", got, want)
	}
	if _, ok := b.Get(2); ok {
		t.Errorf("order 2 still indexed after remove")
	}
}

func TestRemoveLastOrderOfLevel(t *testing.T) {
	b := NewBook()

	b.Insert(newOrder(1, Buy, 102, 10))
	b.Insert(newOrder(2, Buy, 101, 10))
	b.Insert(newOrder(3, Buy, 100, 10))
	checkInvariants(t, b)

	// Middle level.
	if _, ok := b.Remove(2); !ok {
		t.Fatalf("Remove(2) = false, want true")
	}
	checkInvariants(t, b)
	if got, want := levelPrices(b.Bids), []Price{102, 100}; !slices.Equal(got, want) {
		t.Errorf("bid levels = %v, want %v", got, want)
	}

	// Best level: the next one must become best.
	if _, ok := b.Remove(1); !ok {
		t.Fatalf("Remove(1) = false, want true")
	}
	checkInvariants(t, b)
	if best := b.Best(Buy); best == nil || best.Price != 100 {
		t.Errorf("best bid = %v, want level 100", best)
	}
}

func TestRemoveUnknownID(t *testing.T) {
	b := NewBook()

	b.Insert(newOrder(1, Buy, 99, 10))
	b.Insert(newOrder(2, Sell, 101, 10))
	checkInvariants(t, b)

	bids, asks := levelPrices(b.Bids), levelPrices(b.Asks)
	idxLen := len(b.Idx)

	o, ok := b.Remove(42)
	if ok || o != nil {
		t.Fatalf("Remove(42) = %v, %v; want nil, false", o, ok)
	}
	checkInvariants(t, b)

	if got := levelPrices(b.Bids); !slices.Equal(got, bids) {
		t.Errorf("bid levels = %v, want %v", got, bids)
	}
	if got := levelPrices(b.Asks); !slices.Equal(got, asks) {
		t.Errorf("ask levels = %v, want %v", got, asks)
	}
	if len(b.Idx) != idxLen {
		t.Errorf("len(Idx) = %d, want %d", len(b.Idx), idxLen)
	}
}

func TestReduceKeepsQueuePosition(t *testing.T) {
	b := NewBook()
	o1 := newOrder(1, Sell, 100, 10)
	b.Insert(o1)
	b.Insert(newOrder(2, Sell, 100, 10))

	b.Reduce(o1, 4)
	checkInvariants(t, b)

	if o1.Qty != 6 {
		t.Errorf("qty = %d, want 6", o1.Qty)
	}
	if got, want := levelIDs(t, b, Sell, 100), []OrderID{1, 2}; !slices.Equal(got, want) {
		t.Errorf("queue = %v, want %v (partial fill must keep priority)", got, want)
	}
}

func TestReducePanicsOnInvalidQty(t *testing.T) {
	for _, qty := range []Quantity{0, -1, 10, 11} { // 10 = full fill, must use PopBest
		t.Run(fmt.Sprint(qty), func(t *testing.T) {
			b := NewBook()
			o := newOrder(1, Buy, 100, 10)
			b.Insert(o)
			defer func() {
				if recover() == nil {
					t.Errorf("Reduce(%d) did not panic", qty)
				}
			}()
			b.Reduce(o, qty)
		})
	}
}
