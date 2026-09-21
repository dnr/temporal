package fc

import (
	"context"
	"time"
)

type localLimiterTx struct {
	lls    *localLimiterState
	config any
	pri    int32
	age    time.Time
}

func newLocalLimiterTx(
	lls *localLimiterState,
	config any, // FIXME: narrow type earlier
	pri int32,
	age time.Time,
) *localLimiterTx {
	return &localLimiterTx{
		lls:    lls,
		config: config,
		pri:    pri,
		age:    age,
	}
}

func (llt *localLimiterTx) check(cb ReadinessCallback) error {
	return llt.lls.check(llt.config, cb, llt.pri, llt.age)
}

func (llt *localLimiterTx) cancelCheck(cb ReadinessCallback) {
	return llt.lls.cancelCheck(cb)
}

func (llt *localLimiterTx) reserve(context.Context) error {
	return nil // check/cancelCheck is all we need here
}

func (llt *localLimiterTx) commit(context.Context) error {
	return nil // check/cancelCheck is all we need here
}

func (llt *localLimiterTx) cancelReserve(context.Context) {
}
