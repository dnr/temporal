// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package matching

import "sync"

type (
	broadcaster[T any] struct {
		lock   sync.Mutex
		chans  map[chan T]struct{}
		latest T
	}
)

func newBroadcaster[T any]() *broadcaster[T] {
	return &broadcaster[T]{}
}

func (b *broadcaster[T]) Send(value T) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.latest = value
	for ch := range b.chans {
		select {
		case ch <- value:
		default:
			// listeners should read immediately after sending, so this should be rare, but
			// it's better to let a listener miss an update than to block here.
		}
	}
}

func (b *broadcaster[T]) Peek() T {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.latest
}

func (b *broadcaster[T]) Listen() (T, chan T, func()) {
	b.lock.Lock()
	defer b.lock.Unlock()
	ch := make(chan T, 1)
	b.chans[ch] = struct{}{}
	cancel := func() {
		b.lock.Lock()
		defer b.lock.Unlock()
		delete(b.chans, ch)
	}
	return b.latest, ch, cancel
}
