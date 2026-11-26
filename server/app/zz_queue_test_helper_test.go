package app

import "github.com/octarq-org/octarq/server/internal/queue"

func testQueue() queue.Queue {
	q, _ := queue.New("")
	return q
}
