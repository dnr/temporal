package fc

// limiterList is intrusive: list membership needs no allocation beyond the limiter itself.
type limiterList struct {
	head limiterState
	tail limiterState
	len  int
}

func (l *limiterList) pushBack(limiter limiterState) {
	b := limiter.base()
	b.prev = l.tail
	if l.tail != nil {
		l.tail.base().next = limiter
	} else {
		l.head = limiter
	}
	l.tail = limiter
	l.len++
}

func (l *limiterList) remove(limiter limiterState) {
	b := limiter.base()
	if b.prev != nil {
		b.prev.base().next = b.next
	} else {
		l.head = b.next
	}
	if b.next != nil {
		b.next.base().prev = b.prev
	} else {
		l.tail = b.prev
	}
	b.prev, b.next = nil, nil
	l.len--
}
