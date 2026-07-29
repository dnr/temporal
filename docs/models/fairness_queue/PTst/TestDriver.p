// Test drivers for the fair queue model.

machine TestDriver {
  start state Init {
    entry {
      var db: machine;
      var reader: machine;
      var acker: machine;
      var writer: machine;
      var timer: machine;
      var hop: machine;

      // up to 3 injected db timeouts per run
      db = new Database(3);
      timer = new BackoffTimer();
      hop = new Hop();
      // batchSize 2, reload when <= 1 loaded: forces multiple read batches,
      // full-buffer merges, and evictions with few tasks
      // cacheSize 1: tiny evicted-acks cache so trimming is exercised
      reader = new FairReader((db = db, timer = timer, hop = hop, batchSize = 2, reloadAt = 1, cacheSize = 1, initLevel = 0));
      acker = new Acker(reader);
      writer = new FairWriter((db = db, reader = reader, numTasks = 4, maxLevel = 10));
      send reader, eBindAcker, acker;
    }
  }
}

test tcFairQueue [main = TestDriver]:
  assert GuaranteedDelivery, NoLostTask in
    { TestDriver, Database, FairReader, FairWriter, Acker, BackoffTimer, Hop };
