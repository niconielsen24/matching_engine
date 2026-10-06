package engine

import (
	"fmt"
	"slices"
	"sort"
)

type Level struct {
	Price  Price
	Orders []*Order
}

type Book struct {
	Bids []*Level
	Asks []*Level
	Idx  map[OrderID]*Order
}

func NewBook() *Book {
	return &Book{
		Bids: []*Level{},
		Asks: []*Level{},
		Idx:  make(map[OrderID]*Order),
	}
}

// Mutations
func (b *Book) levels(side Side) *[]*Level {
	if side == Buy {
		return &b.Bids
	}
	return &b.Asks
}

func (b *Book) findLevel(side Side, price Price) (int, bool) {
	levels := b.levels(side)
	i := sort.Search(len(*levels), func(i int) bool {
		if side == Buy {
			return (*levels)[i].Price <= price
		}
		return (*levels)[i].Price >= price
	})
	return i, i < len(*levels) && (*levels)[i].Price == price
}

func (b *Book) Insert(order *Order) {
	levels := b.levels(order.Side)
	i, found := b.findLevel(order.Side, order.Price)
	if !found {
		level := &Level{
			Price:  order.Price,
			Orders: []*Order{order},
		}
		*levels = slices.Insert(*levels, i, level)
	} else {
		(*levels)[i].Orders = append((*levels)[i].Orders, order)
	}

	b.Idx[order.OrderID] = order
}

func (b *Book) Remove(orderID OrderID) (*Order, bool) {
	order, ok := b.Idx[orderID]
	if !ok {
		return nil, false
	}

	levels := b.levels(order.Side)
	i, found := b.findLevel(order.Side, order.Price)
	if !found {
		panic(fmt.Sprintf("remove: level not found for order %d", orderID))
	}
	level := (*levels)[i]
	j := slices.Index(level.Orders, order)
	if j < 0 {
		panic(fmt.Sprintf("remove: order %d not found in level %d", orderID, order.Price))
	}
	level.Orders = slices.Delete(level.Orders, j, j+1)
	if len(level.Orders) == 0 {
		*levels = slices.Delete(*levels, i, i+1)
	}

	delete(b.Idx, orderID)
	return order, true
}

// PopBest returns the best order from the given side.
// For matching use the opposite side, e.g. to match a buy order, pop the best sell order.
func (b *Book) PopBest(side Side) (*Order, bool) {
	levels := b.levels(side)
	if len(*levels) == 0 {
		return nil, false
	}
	level := (*levels)[0]
	order := level.Orders[0]
	level.Orders[0] = nil
	level.Orders = level.Orders[1:]
	if len(level.Orders) == 0 {
		(*levels)[0] = nil
		*levels = (*levels)[1:]
	}

	delete(b.Idx, order.OrderID)
	return order, true
}
func (b *Book) Reduce(order *Order, qty Quantity) {
	if qty >= order.Qty || qty <= 0 {
		panic(fmt.Sprintf("reduce: invalid qty %d for order %d (has %d)", qty, order.OrderID, order.Qty))
	}
	order.Qty -= qty
}

// Queries
func (b *Book) Best(side Side) *Level {
	levels := b.levels(side)
	if len(*levels) == 0 {
		return nil
	}
	return (*levels)[0]
}
func (b *Book) Get(orderID OrderID) (*Order, bool) {
	order, ok := b.Idx[orderID]
	return order, ok
}
