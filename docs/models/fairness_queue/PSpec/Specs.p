// Specs for the fair queue model.

// GuaranteedDelivery (liveness): every task whose write was confirmed to the
// writer is eventually completed (acked) by the reader. Hot while any
// confirmed task is un-acked; a schedule that ends hot is a liveness bug
// (covers both lost tasks and a stuck reader).
//
// A task can be completed before the writer sees the write confirmation (a
// concurrent read can load it while the write response is still in flight),
// so track early acks separately.
spec GuaranteedDelivery observes eTaskConfirmed, eTaskCompleted {
  var pending: set[int];    // confirmed but not yet completed
  var ackedEarly: set[int]; // completed before we saw the confirmation

  start cold state AllCompleted {
    on eTaskConfirmed do (lvl: int) {
      handleConfirmed(lvl);
      if (sizeof(pending) > 0) {
        goto PendingCompletion;
      }
    }
    on eTaskCompleted do (lvl: int) {
      handleCompleted(lvl);
    }
  }

  hot state PendingCompletion {
    on eTaskConfirmed do (lvl: int) {
      handleConfirmed(lvl);
    }
    on eTaskCompleted do (lvl: int) {
      handleCompleted(lvl);
      if (sizeof(pending) == 0) {
        goto AllCompleted;
      }
    }
  }

  fun handleConfirmed(lvl: int) {
    if (lvl in ackedEarly) {
      ackedEarly -= (lvl);
    } else {
      pending += (lvl);
    }
  }

  fun handleCompleted(lvl: int) {
    if (lvl in pending) {
      pending -= (lvl);
    } else {
      ackedEarly += (lvl);
    }
  }
}
