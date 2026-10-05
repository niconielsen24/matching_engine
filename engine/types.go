package engine

type Price = int64
type Quantity = int64
type OrderID = int64

type Side = int8

const (
	Buy Side = iota
	Sell
)

type OrderType = int8

const (
	Limit OrderType = iota
	Market
)

type TIF uint8 // Time in Force

const (
	GTC TIF = iota
	IOC
	FOK
)

type CmdKind = int8

const (
	New CmdKind = iota
	Cancel
)

type Command struct {
	Seq      uint64  // assigned by sequencer; engine never invents it
	Kind     CmdKind // New, Cancel
	ID       OrderID
	Side     Side
	Type     OrderType
	TIF      TIF
	PostOnly bool
	Price    Price
	Qty      Quantity
}

type EventKind = int8

const (
	Accepted EventKind = iota
	Rejected
	Fill
	Cancelled
)

type RejectReason = int8

const (
	InvalidPrice RejectReason = iota
	InvalidQty
	InvalidTIF
	InvalidOrderID
	InvalidSide
	InvalidType
	PostOnlyRejected
)

type Event struct {
	Seq          uint64
	Kind         EventKind // Accepted, Rejected, Fill, Cancelled
	Reason       RejectReason
	Maker, Taker OrderID
	Price        Price
	Qty          Quantity
}

type Order struct {
	OrderID OrderID
	Side    Side
	Type    OrderType
	Price   Price
	Qty     Quantity
}
