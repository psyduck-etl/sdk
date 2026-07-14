package rpc

const (
	// batchItems caps how many data items one Batch message carries. It is
	// also the buffer given to rpc-owned data channels, so a resource can
	// run ahead of the wire and batches actually form under load.
	batchItems = 256

	// batchBytes soft-caps a Batch's cumulative payload, keeping messages
	// well under gRPC's default 4 MiB receive ceiling. A single oversized
	// item still crosses alone — the cap is checked between items.
	batchBytes = 1 << 20
)

// gather starts a batch with first and greedily drains whatever is already
// pending on ch. It never blocks: batches form under load without adding
// latency when idle. A close observed mid-gather just ends the batch — the
// caller's next receive sees the close.
func gather(first []byte, ch <-chan []byte) [][]byte {
	items := [][]byte{first}
	size := len(first)
	for len(items) < batchItems && size < batchBytes {
		select {
		case b, ok := <-ch:
			if !ok {
				return items
			}
			items = append(items, b)
			size += len(b)
		default:
			return items
		}
	}
	return items
}
