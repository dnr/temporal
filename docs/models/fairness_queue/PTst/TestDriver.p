// Test drivers for the fair queue model.

machine TestDriver {
  start state Init {
    entry {
      var db: machine;
      var reader: machine;
      var acker: machine;
      var writer: machine;

      db = new Database();
      // batchSize 2, reload when <= 1 loaded: forces multiple read batches
      // and full-buffer handling with few tasks
      reader = new FairReader((db = db, batchSize = 2, reloadAt = 1, initLevel = 0));
      acker = new Acker(reader);
      writer = new FairWriter((db = db, reader = reader, numTasks = 4, startLevel = 1));
      send reader, eBindAcker, acker;
    }
  }
}

test tcHappyPath [main = TestDriver]:
  assert GuaranteedDelivery in { TestDriver, Database, FairReader, FairWriter, Acker };
