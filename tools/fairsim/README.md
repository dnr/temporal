# fairsim - Fair Task Queue Simulator

A tool for simulating task queue fairness behavior using Temporal's fairness algorithms. This simulator helps analyze how different task priorities, fairness keys, and weights affect task scheduling and latency.

## Overview

The simulator creates tasks with configurable fairness keys and weights, processes them through a priority queue system with fairness counters, and outputs detailed statistics about task latencies and fairness metrics.

## Usage

fairsim supports two modes: **default mode** (generates tasks) and **script mode** (executes commands from a file).

### Command Line Syntax

```bash
fairsim [main-flags] [-- gentasks-flags]
fairsim [main-flags] -- -script=<file>
```

### Main Flags

These flags configure the simulator itself:

- `-seed=<int>` - Random seed (default: random)  
- `-fair=<bool>` - Enable fairness algorithm (default: true, false = FIFO)
- `-partitions=<int>` - Number of task queue partitions (default: 4)
- `-counter-params=<file>` - JSON file with fairness counter parameters
- `-script=<file>` - Execute script file instead of generating tasks

## Default Mode (Task Generation)

By default, fairsim generates tasks using a Zipf distribution and processes them through the fairness algorithm.

### Basic Usage

```bash
# Generate 100 tasks with 10 unique fairness keys (default)
fairsim

# Generate with custom parameters  
fairsim -- -tasks=1000 -keys=50 -keyprefix=test

# Disable fairness (FIFO mode)
fairsim -fair=false -- -tasks=500
```

### Task Generation Flags (after `--`)

- `-tasks=<int>` - Number of tasks to generate (default: 100)
- `-keys=<int>` - Number of unique fairness keys (default: 10)  
- `-keyprefix=<string>` - Prefix for generated fairness keys (default: "key")
- `-zipf_s=<float>` - Zipf distribution s parameter (default: 2.0)
- `-zipf_v=<float>` - Zipf distribution v parameter (default: 2.0)

## Script Mode

Script mode allows you to execute a sequence of commands to precisely control task creation and processing.

```bash
fairsim -- -script=myscript.txt
```

### Script Commands

Scripts support these commands (one per line):

#### `task` - Add a single task
```bash
task [-fkey=<key>] [-fweight=<weight>] [-pri=<priority>] [-payload=<payload>]
```

- `-fkey=<string>` - Fairness key (default: "default")
- `-fweight=<float>` - Fairness weight (default: 1.0)  
- `-pri=<int>` - Priority (default: 3)
- `-payload=<string>` - Arbitrary payload for identification

#### `poll` - Process next task
```bash
poll
```
Removes and prints the next task from the queue.

#### `gentasks` - Generate multiple tasks  
```bash
gentasks -tasks=<num> -keys=<num> [-keyprefix=<string>] [-zipf_s=<float>] [-zipf_v=<float>]
```
Same parameters as default mode.

#### `stats` - Print current statistics
```bash
stats
```
Shows latency statistics for tasks processed so far.

#### `clearstats` - Reset statistics
```bash
clearstats  
```
Clears all recorded latency data.

### Example Script

```bash
# script.txt
gentasks -tasks=50 -keys=5 -keyprefix=batch1
poll
poll
stats

task -fkey=priority1 -pri=1 -payload="high priority"
task -fkey=priority5 -pri=5 -payload="low priority"  
poll
poll

clearstats
gentasks -tasks=20 -keys=3
```

Comments (lines starting with `#`) and empty lines are ignored.

## Output

### Task Output Format
```
task idx-dsp:     0-     1 =      0  pri: 3  fkey:    "key1"  fweight:  1  part: 2  payload:"test"
```

- `idx-dsp`: task index when created - dispatch order  
- Latency: dispatch order - creation index
- `pri`: task priority
- `fkey`: fairness key
- `fweight`: fairness weight  
- `part`: partition where task was processed
- `payload`: user-defined payload

### Statistics

The tool outputs comprehensive fairness metrics:

- **Raw Latency Statistics**: Basic latency percentiles
- **Normalized Latency Statistics**: Latencies normalized by task count per key
- **Per-key Statistics**: Detailed breakdown by fairness key
- **Fairness Metrics**: Cross-key percentile analysis

## Examples

```bash
# Basic simulation
fairsim

# Large simulation with custom seed
fairsim -seed=12345 -- -tasks=10000 -keys=100

# Test unfair vs fair behavior  
fairsim -fair=false -- -tasks=1000 > unfair.out
fairsim -fair=true -- -tasks=1000 > fair.out

# Custom script execution
echo "gentasks -tasks=10 -keys=2
poll
poll  
stats" > test.txt
fairsim -- -script=test.txt

# Multiple partitions
fairsim -partitions=8 -- -tasks=5000 -keys=200
```

## Algorithm Details

The simulator implements Temporal's fairness algorithm using:

- **Priority Queues**: Tasks ordered by (priority, fairness_pass, creation_index)
- **Fairness Counters**: Track fairness "debt" per key to ensure balanced processing  
- **Partitioning**: Distributes tasks across partitions for scalability
- **Zipf Distribution**: Models realistic key popularity patterns

The fairness algorithm ensures that tasks with the same priority are processed proportionally to their fairness weights, preventing starvation of less popular keys.