package engine

type Engine struct {
	book *Book
}

func NewEngine() *Engine {
	return &Engine{
		book: NewBook(),
	}
}

func (e *Engine) Apply(cmd Command, out []Event) []Event {
	switch cmd.Kind {
	case New:
		return e.handleNew(cmd, out)
	case Cancel:
		return e.handleCancel(cmd, out)
	default:
		return append(out, reject(cmd, UnknownCommand))
	}
}

func (e *Engine) handleNew(cmd Command, out []Event) []Event {
	valid, reason := e.validateNew(cmd)
	if !valid {
		return append(out, reject(cmd, reason))
	}

	// publish accepted event
	out = append(out, Event{
		Seq:   cmd.Seq,
		Kind:  Accepted,
		Taker: cmd.ID,
	})

	// Match as taker if possible, and return remaining quantity and events
	remaining, out := e.match(cmd, out)

	// If there is remaining quantity, add to book
	// Now is taker
	if remaining > 0 {
		e.book.Insert(&Order{
			OrderID: cmd.ID,
			Side:    cmd.Side,
			Type:    cmd.Type,
			Price:   cmd.Price,
			Qty:     remaining,
		})
	}

	return out
}

func (e *Engine) handleCancel(cmd Command, out []Event) []Event {
	order, ok := e.book.Remove(cmd.ID)
	if !ok {
		return append(out, reject(cmd, InvalidOrderID))
	}

	out = append(out, Event{
		Seq:   cmd.Seq,
		Kind:  Cancelled,
		Maker: cmd.ID,
		Taker: cmd.ID,
		Price: order.Price,
		Qty:   order.Qty,
	})
	return out
}

func reject(cmd Command, reason RejectReason) Event {
	return Event{
		Seq:    cmd.Seq,
		Kind:   Rejected,
		Reason: reason,
	}
}

func (e *Engine) validateNew(cmd Command) (bool, RejectReason) {
	if _, ok := e.book.Get(cmd.ID); ok {
		return false, DuplicateOrderID
	}
	if cmd.Price <= 0 {
		return false, InvalidPrice
	}
	if cmd.Qty <= 0 {
		return false, InvalidQty
	}
	if cmd.TIF == TIFunset {
		return false, InvalidTIF
	}
	return true, 0
}

func (e *Engine) validateCancel(cmd Command) (bool, RejectReason) {
	if _, ok := e.book.Get(cmd.ID); !ok {
		return false, InvalidOrderID
	}
	return true, 0
}

func (e *Engine) match(cmd Command, out []Event) (Quantity, []Event) {
	remaining := cmd.Qty
	oppositeSide := Buy
	if cmd.Side == Buy {
		oppositeSide = Sell
	}

	best := e.book.Best(oppositeSide)
	for remaining > 0 && best != nil && crosses(cmd, best.Price) {
		maker := best.Orders[0]
		fillQty := min(remaining, maker.Qty)
		out = append(out, Event{
			Seq:   cmd.Seq,
			Kind:  Fill,
			Maker: maker.OrderID,
			Taker: cmd.ID,
			Price: best.Price,
			Qty:   fillQty,
		})
		if fillQty == maker.Qty {
			e.book.PopBest(oppositeSide)
		} else {
			e.book.Reduce(maker, fillQty)
		}
		remaining -= fillQty
		best = e.book.Best(oppositeSide)
	}

	return remaining, out
}

func crosses(cmd Command, price Price) bool {
	if cmd.Side == Buy {
		return cmd.Price >= price
	}
	return cmd.Price <= price
}
