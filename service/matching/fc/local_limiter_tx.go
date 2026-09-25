package fc

import (
	"context"
	"time"

	"go.temporal.io/server/service/matching/simplelimiter"
)

type localLimiterTx struct {
	lls    *localLimiterState
	config simplelimiter.Params
	pri    int32
	age    time.Time
}

func newLocalLimiterTx(lls *localLimiterState, config simplelimiter.Params, pri int32, age time.Time) *localLimiterTx {
	return &localLimiterTx{lls: lls, config: config, pri: pri, age: age}
}

func (llt *localLimiterTx) check(cb ReadinessCallback) error {
	return llt.lls.check(llt.config, cb, llt.pri, llt.age)
}

func (llt *localLimiterTx) cancelCheck() {
	llt.lls.cancelCheck()
}

func (llt *localLimiterTx) reserve(context.Context) error {
	return nil // check/cancelCheck is all we need here
}

func (llt *localLimiterTx) commit(context.Context) error {
	return nil // check/cancelCheck is all we need here
}

func (llt *localLimiterTx) cancelReserve(context.Context) {
}
