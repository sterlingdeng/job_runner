package jobs

import "sync/atomic"

type ider struct { id int32 }

func (i *ider) nextID() int32 {
	return atomic.AddInt32(&i.id, 1)
}
