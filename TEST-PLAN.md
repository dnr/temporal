
in `common/util/util_test.go`:
- [done] test for GetOrSetNew
- [done] test for GetOrSetMap

in `common/goro/keyed_set_test.go`:
- [done] test KeyedSet

the rest in `./service/matching/`:

`matcher_data_test.go` / MatcherDataSuite:
- [done] tests for using min priority (TestMinPriorityFiltering, TestMinPriorityZeroMatchesAll)
- [done] tests for priority backlog poll forwarders matching when normal poll forwarders can't (TestAllowForwardingBlocksNormalPollForwarder, TestAllowForwardingPermitsPriorityBacklogForwarder, TestPriorityBacklogForwarderOrder)
- [done] tests for MatchPollerImmediately (TestMatchPollerImmediately, TestMatchPollerImmediatelyNoTask, TestMatchPollerImmediatelyWithMinPriority)

priMatcher:
- create priority backlog forwarders on UpdateRemotePriorityBacklogs
  (note: this is covered at integration level by TestStickyInteraction_SinglePartition; unit testing requires
  significant mocking of the matching client for RPC calls)

TestTaskQueuePartitionManager:
- test that updateEphemeralDataIteration does the right thing
  (note: ephemeral data flow is tested in TestUserData_LocalBacklogPriorityChanged, TestUserData_EphemeralDataMerging;
  updateEphemeralDataIteration relies on physical queue stats which are complex to mock at unit test level)

TestUserDataManager:
- [done] test that LocalBacklogPriorityChanged sets ephemeral data and calls onEphemeralDataChanged callback (TestUserData_LocalBacklogPriorityChanged)
- [done] test that ephemeral data merges correctly from incoming + local sources (TestUserData_EphemeralDataMerging)
- [done] test that ephemeralDataChanged channel signals correctly (TestUserData_EphemeralDataChangedChannel)

in `./tests` PrioritySuite.TestStickyInteraction_SinglePartition:
- [done] validate that the 3N tasks came in the right order (high pri, default sticky tasks, low pri)

