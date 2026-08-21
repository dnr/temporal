package tests

/*
test plan:

one test for each of {workflow task, activity task, standalone (chasm) activity task}

set whole-queue concurrency limit to 5
queue up 50 tasks
start 10 workers with options:
	maxconcurrentworkflowtasks = 1
	maxconcurrentactivitytasks = 1
have the task implementation (for whatever task type) sleep for a random value between 0-1 second
also have it record how many are running concurrently by incrementing/decrementing an atomic int before/after the sleep
record maximum concurrency reached
ensure all tasks are processed (should take 5-10 seconds)
check that max concurrency reached 5 and did not exceed it

to queue up workflow activity tasks, you can create one workflow and respond to its first wft
with taskpoller instead of creating a workflow worker.

follow the latest guidelines for writing functional tests (I think it's parallelsuite but check that).

the linter will warn about time.Sleep(), silence it with a comment, I think it's appropriate here.

*/
