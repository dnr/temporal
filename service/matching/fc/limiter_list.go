package fc

// limiterGCList is intrusive: list membership needs no allocation beyond the limiter itself.
type limiterGCList struct {
	head *limiterStateBase
	tail *limiterStateBase
	len  int
}

func (l *limiterGCList) pushBack(b *limiterStateBase) {
	b.prev = l.tail
	if l.tail != nil {
		l.tail.next = b
	} else {
		l.head = b
	}
	l.tail = b
	l.len++
}

func (l *limiterGCList) remove(b *limiterStateBase) {
	if b.prev != nil {
		b.prev.next = b.next
	} else {
		l.head = b.next
	}
	if b.next != nil {
		b.next.prev = b.prev
	} else {
		l.tail = b.prev
	}
	b.prev, b.next = nil, nil
	l.len--
}
