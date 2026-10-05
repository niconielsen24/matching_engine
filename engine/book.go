package engine

import (
	"slices"
	"sort"
)

type Level struct {
	Price  Price
	Orders []*Order
}

type Book struct {
	Bids []Level
	Asks []Level
	Idx  map[OrderID]*Order
}

func NewBook() *Book {
	return &Book{
		Bids: []Level{},
		Asks: []Level{},
		Idx:  make(map[OrderID]*Order),
	}
}

// Mutations
func (b *Book) Insert(order *Order) {
	b.Idx[order.OrderID] = order
	if order.Side == Buy {
		b.insertBid(order)
	} else {
		b.insertAsk(order)
	}
}

func (b *Book) insertBid(order *Order) {
	i := sort.Search(len(b.Bids), func(i int) bool {
		return b.Bids[i].Price <= order.Price
	})
	if i < len(b.Bids) && b.Bids[i].Price == order.Price {
		b.Bids[i].Orders = append(b.Bids[i].Orders, order)
	} else {
		level := Level{
			Price:  order.Price,
			Orders: []*Order{order},
		}
		b.Bids = slices.Insert(b.Bids, i, level)
	}
}

func (b *Book) insertAsk(order *Order) {
	i := sort.Search(len(b.Asks), func(i int) bool {
		return b.Asks[i].Price >= order.Price
	})
	if i < len(b.Asks) && b.Asks[i].Price == order.Price {
		b.Asks[i].Orders = append(b.Asks[i].Orders, order)
	} else {
		level := Level{
			Price:  order.Price,
			Orders: []*Order{order},
		}
		b.Asks = slices.Insert(b.Asks, i, level)
	}
}

func (b *Book) Remove(orderID OrderID) (*Order, bool)    { panic("unimplemented") }
func (b *Book) PopBest(side Side) (*Order, bool)         { panic("unimplemented") }
func (b *Book) Reduce(orderID *Order, qty Quantity) bool { panic("unimplemented") }

// Queries
func (b *Book) Best(side Side) *Level              { panic("unimplemented") }
func (b *Book) Get(orderID OrderID) (*Order, bool) { panic("unimplemented") }
