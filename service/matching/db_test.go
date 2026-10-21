package matching

func (db *taskQueueDB) setMaxReadLevelForTesting(subqueue subqueueIndex, level int64) {
	db.Lock()
	defer db.Unlock()
	db.subqueues[subqueue].maxReadLevel = level
}
