
in `common/util/util_test.go`:
- test for GetOrSetNew
- test for GetOrSetMap

in `common/goro/keyed_set_test.go`:
- test KeyedSet

the rest in `./service/matching/`:

`matcher_data_test.go` / MatcherDataSuite:
- tests for using min priority
- test that "priority backlog poll forwarders" can match even if normal poll forwarders can't (it looks like we don't have any current tests for the "allowForwarding" state blocking poll forwarding, so add that too)
- tests for MatchPollerImmediately

priMatcher:
- create priority backlog forwarders on UpdateMaxPriorityBacklogs

TestTaskQueuePartitionManager:
- test that updateEphemeralDataIteration does the right thing

TestUserDataManager:
- test that ephemeral data propagates downward when it changes on the root
- test that MaxPriorityBacklogChanged sets ephemeral data and propagates it to children
- test that MaxPriorityBacklogChanged on a child merges that new data with data it got from the root and sends the correct merged data to _its_ child

in `./tests` PrioritySuite.TestStickyInteraction_SinglePartition:
- validate that the 3N tasks came in the right order (high pri, default sticky tasks, low pri)

