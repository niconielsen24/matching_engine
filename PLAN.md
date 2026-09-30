# Matching Engine in Go — Project Plan

A limit order book and matching engine, built in Go, measured honestly, and documented as a portfolio piece for systems / low-latency roles.

**Goal:** a correct, deterministic engine first; fast second; every optimization backed by numbers.
**Out of scope for now:** Rust, multiple markets, margin/risk, a UI.

---

## 0. Concepts you need first (1–2 days)

You don't need prior experience; these ideas cover 90% of it.

- **Order book:** two sides. *Bids* (buy orders) sorted highest price first, *asks* (sell orders) sorted lowest price first. The gap between the best bid and best ask is the *spread*.
- **Price-time priority:** better price wins; at the same price, the order that arrived first wins (FIFO per price level).
- **Maker vs taker:** an order that rests in the book is a *maker*. An incoming order that matches against resting orders is a *taker*. Trades always execute at the **maker's** price.
- **Crossing:** a buy limit at a price ≥ best ask (or a sell ≤ best bid) crosses the spread and matches immediately. Whatever quantity is left rests in the book.
- **Order types to support (in this order):**
  - *Limit* — match what crosses, rest the remainder.
  - *Cancel* — remove a resting order by ID.
  - *Market* — match at any price until filled or the book runs out; never rests.
  - *IOC* (immediate-or-cancel) — like limit, but the unfilled remainder is cancelled instead of resting.
  - *Post-only* — must be a maker; if it would cross, reject it.
  - *FOK* (fill-or-kill) — fill completely right now or do nothing. (Optional, later.)
- **Determinism:** same inputs in the same order → exactly the same outputs, every time. Real exchanges depend on this for replay, recovery, and auditing.

Worth skimming: Nexus's own exchange docs (<https://docs.nexus.xyz/exchange>) — you'll see post-only, batch quoting, and cancel-all, which tells you what they care about.

Exercise before coding: on paper, take an empty book, apply ~10 orders (limits on both sides, one crossing order, one cancel, one market order) and write down the book and trades after each step. These become your first test cases.

---

## 1. Architecture

**Engine as a library, not a framework.** The core package has no I/O, no goroutines, no clock. It's a pure state machine: `command in → events out`. Everything else (network, logging, persistence, benchmarks) is composed around it.

```
               ┌─────────────┐     commands     ┌──────────────┐    events    ┌─────────────┐
 clients ───▶  │  gateway    │ ───────────────▶ │  sequencer   │ ───────────▶ │   engine    │
 (TCP/WS)      │ (parse,     │                  │ (assign seq, │              │ (pure, one  │
               │  validate)  │                  │  append WAL) │              │  goroutine) │
               └─────────────┘                  └──────────────┘              └──────┬──────┘
                                                                                     │ events
                                                                  ┌──────────────────┼────────────────┐
                                                                  ▼                  ▼                ▼
                                                             order acks        market data       event log
```

Phases 1–5 only build the `engine` box plus tests and benchmarks. The rest comes in phases 6–7.

### Repo layout

```
matching-engine/
├── engine/          # pure core: types, book, matching. No I/O.
│   ├── types.go
│   ├── book.go
│   ├── engine.go
│   └── *_test.go
├── internal/gen/    # seeded synthetic order-flow generator
├── cmd/
│   ├── bench/       # load run: throughput + latency histogram
│   ├── replay/      # replay an input log, print/hash events
│   └── server/      # (phase 6) TCP/WS gateway
├── testdata/        # golden input/output logs
└── README.md        # architecture, benchmark tables, what I learned
```

### Core types (sketch)

```go
type Price int64   // in ticks, never float
type Qty   int64   // in lots, never float
type OrderID uint64

type Side uint8
const ( Buy Side = iota; Sell )

type OrderType uint8
const ( Limit OrderType = iota; Market )

type TIF uint8
const ( GTC TIF = iota; IOC; FOK )

type Command struct {
    Seq      uint64    // assigned by sequencer; engine never invents it
    Kind     CmdKind   // New, Cancel
    ID       OrderID
    Side     Side
    Type     OrderType
    TIF      TIF
    PostOnly bool
    Price    Price
    Qty      Qty
}

type Event struct {
    Seq    uint64
    Kind   EventKind   // Accepted, Rejected, Fill, Cancelled
    Reason RejectReason
    Maker, Taker OrderID
    Price  Price
    Qty    Qty
}

// The whole public API of the core:
func (e *Engine) Apply(cmd Command, out []Event) []Event
```

`Apply` appends to a caller-provided slice so the hot path can later run with zero allocations.

---

## 2. Phases

Each phase ends with something that works, tests that pass, and a note in `NOTES.md` about what you learned. Time estimates assume part-time work alongside university.

### Phase 1 — Naive but correct (week 1)

- Book per side as a **sorted slice of price levels**; each level holds a slice of orders (FIFO).
- A `map[OrderID]*order` index so cancels can find orders.
- Support: limit (GTC) and cancel. Partial fills.
- **Tests:** table-driven tests from your paper exercise. Each case = list of commands → expected events + expected final book.

Done when: all paper scenarios pass and you can print the book.

### Phase 2 — Complete order types (week 2)

- Market orders, IOC, post-only. FOK optional.
- Reject reasons: unknown order on cancel, zero/negative qty, post-only would cross, market order into empty book.
- **Invariant checks** (run after every command in tests):
  - Book is never crossed (best bid < best ask).
  - Every level is non-empty; no order with qty ≤ 0 rests.
  - Quantity conservation: submitted = filled + resting + cancelled.
  - Index and book agree (every indexed order is in the book and vice versa).
- **Fuzzing:** `go test -fuzz` feeding random command sequences, asserting the invariants. This finds bugs you'd never write tests for.

Done when: fuzzing runs for several minutes with no invariant violations.

### Phase 3 — Determinism and replay (week 3)

- Input log format: one command per line (JSON is fine to start; switch to binary later if you want).
- `cmd/replay`: reads an input log, runs the engine, writes the event log, prints a hash (e.g. SHA-256) of all events.
- **Golden tests:** store input + expected output in `testdata/`; the test fails if output changes.
- Determinism test: run the same 1M-command log twice → identical hash.

Things that silently break determinism — keep them out of `engine/`:
- Iterating over a Go `map` (order is randomized). Use the map only for lookup.
- `time.Now()` — timestamps come from the sequencer, inside the command.
- Goroutines or anything concurrent inside the core.
- Floats.

Done when: the replay hash is stable across runs and machines.

### Phase 4 — Baseline measurements (week 4)

Measure the naive version **before** optimizing anything. This is the "before" column of every table in your README.

- `internal/gen`: seeded generator producing realistic flow — most orders near the mid price, a mix of ~60% limit / 30% cancel / 10% market (tune it; document your choice).
- Go benchmarks: `go test -bench=. -benchmem` → ns/op, B/op, allocs/op per command.
- `cmd/bench`: runs N million commands, reports throughput (commands/sec) and **latency percentiles** (p50, p99, p99.9, max) using an HDR histogram (e.g. `github.com/HdrHistogram/hdrhistogram-go`).
- Profiles: `pprof` CPU and heap; `go tool trace` for GC pauses.
- Record machine specs and Go version next to every number.

Done when: you have a baseline table and a flame graph showing where time goes.

### Phase 5 — Optimization rounds (weeks 5–7)

**One change at a time, re-measure, record the result — including changes that didn't help.** Candidates, roughly in order of expected impact:

1. **Price level structure.** Sorted slice → balanced tree (e.g. `github.com/google/btree`) or, if prices are bounded, a flat array indexed by tick with a best-price pointer. Compare all of them.
2. **Allocations.** Pool order structs (a free list you manage, or `sync.Pool`), preallocate slices, reuse the `out []Event` buffer. Target: 0 allocs/op on the hot path. Show the effect on p99.9 and GC time.
3. **Memory layout.** Store orders in one big slice and link them with `int32` indices instead of pointers (intrusive doubly linked list per level). Better cache locality, less GC scanning. Cancel becomes O(1).
4. **Struct packing.** Order fields by size to shrink structs; check with `unsafe.Sizeof`.
5. **GC tuning** (only after the above): `GOGC`, `GOMEMLIMIT`. Document it as a trade-off, not a fix.

Suggested README table format:

| Version | Throughput (cmd/s) | p50 | p99 | p99.9 | allocs/op | Notes |
|---|---|---|---|---|---|---|
| v1 naive slices | | | | | | baseline |
| v2 btree levels | | | | | | |
| v3 pooled orders | | | | | | |
| v4 index-linked lists | | | | | | |

Done when: you've run at least 3 optimization rounds with numbers and a sentence of analysis each.

### Phase 6 — Gateway and end-to-end latency (weeks 8–9)

- `cmd/server`: TCP (simple length-prefixed binary or newline JSON) and/or WebSocket.
- One goroutine per connection parses and validates → a channel into the **sequencer** goroutine → engine goroutine → fan-out goroutine for acks and market data.
- Load generator client with many connections.
- Measure end-to-end latency (client send → ack received) and compare with in-process engine latency. The gap is your network + channel + serialization cost; profile it.
- Experiment: channel vs a ring buffer between stages. Measure, don't assume.

### Phase 7 — Persistence and recovery (week 10, stretch)

- Sequencer appends every command to a write-ahead log before the engine sees it.
- Periodic snapshot of book state.
- Recovery = load latest snapshot + replay WAL after it → verify the event hash matches the original run.
- Measure the cost of `fsync` per command vs batched fsync. Classic latency vs durability trade-off — great interview material.

### Phase 8 — Write-up (ongoing)

- README: what it is, architecture diagram, how to run, benchmark tables, what didn't work.
- One LinkedIn post per interesting finding (e.g. "zero allocations cut p99.9 by X", "the tree I expected to win lost to a flat array").
- Keep `NOTES.md` as a running log while you build; the posts write themselves from it.

---

## 3. Testing strategy (summary)

| Layer | Tool | Purpose |
|---|---|---|
| Scenarios | table-driven tests | known cases from paper exercises |
| Invariants | checked after every command | catch corrupt state immediately |
| Random input | `go test -fuzz` | find edge cases you didn't think of |
| Regression | golden files in `testdata/` | output never changes by accident |
| Determinism | replay twice, compare hash | proves the core is a pure state machine |
| Performance | `-bench -benchmem`, HDR histogram, pprof | numbers behind every claim |

Run tests with `-race` from Phase 6 onward, once goroutines exist.

---

## 4. Common pitfalls

- **Floats for money.** Use integer ticks and lots from day one.
- **Optimizing before measuring.** Phase 4 exists so every later claim has a baseline.
- **Averages instead of percentiles.** Report p99/p99.9; trading systems care about the tail.
- **Benchmarking a warm, tiny book.** Test with realistic book depth (thousands of resting orders) and cancel-heavy flow — cancels are most of real exchange traffic.
- **Concurrency inside the matcher.** A single-threaded core is the industry norm for a reason: determinism and no locks. Parallelism belongs around it (per-market engines, I/O stages).
- **Scope creep.** Multiple markets, margin, self-trade prevention, and a UI are all interesting and all later.

---

## 5. Interview talking points this project gives you

- Why the core is single-threaded, and where concurrency lives instead.
- How you guaranteed determinism and proved it.
- Data structure choices for the book and what the benchmarks said.
- How allocation and memory layout affected tail latency in Go.
- The durability vs latency trade-off in the WAL.
- What you'd move to Rust first, and why (hint: the core, once the Go version is well measured, gives you a fair comparison).