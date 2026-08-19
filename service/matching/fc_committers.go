package matching

type fcCommitter interface {
	Reserve() error
	CancelReservations()
	Commit() error
}

type fcConcurrencyCommitter struct {
	task *internalTask
	key  string
}

func (c *fcConcurrencyCommitter) Reserve() error {
	panic("not implemented") // FIXME: Implement
}

func (c *fcConcurrencyCommitter) CancelReservations() {
	panic("not implemented") // FIXME: Implement
}

func (c *fcConcurrencyCommitter) Commit() error {
	panic("not implemented") // FIXME: Implement
}
