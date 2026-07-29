// Test drivers for the fair queue model.

machine TestDriver {
  start state Init {
    entry {
      var db: machine;
      var reader: machine;
      var acker: machine;
      var writer: machine;

      db = new Database();
      // batchSize 2, reload when <= 1 loaded: forces multiple read batches,
      // full-buffer merges, and evictions with few tasks
      reader = new FairReader((db = db, batchSize = 2, reloadAt = 1, initLevel = 0));
      acker = new Acker(reader);
      writer = new FairWriter((db = db, reader = reader, numTasks = 4, maxLevel = 8));
      send reader, eBindAcker, acker;
    }
  }
}

test tcFairQueue [main = TestDriver]:
  assert GuaranteedDelivery in { TestDriver, Database, FairReader, FairWriter, Acker };
