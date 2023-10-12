# Horus Control Plane

This repository contains our implementation of the control plane of the NSDI’24 paper, **Horus: Granular In-Network Task Scheduler for Cloud Datacenters**.

Horus has both data plane and control plane components. 

The control plane of Horus consists of a centralized controller, spine controllers, and leaf controllers. While the data plane schedules tasks, the control plane handles initialization, failures, and dynamic virtual clusters.

## Building

Dependencies:
- golang 1.18

We provide few scripts to install the dependencies (more testing is required):

    $ cd scripts/env
    $ sudo ./0_install_base_deps.sh # IMPORTANT: will remove the current golang version!
    $ ./1_set_go_path.sh
    $ ./2_install_deps.sh

Build:

    $ make all

## Main Assumptions
We made few assumptions while designing the control plane. Most of these assumptions are implementation-specific (i.e., not inherent to Horus). This means that we can get rid off them in the future.

1. ***Design-specific Assumptions***
    - The centralized controller doesn't fail
2. ***Implementation-specific Assumptions***
    - The spine controllers don't fail
    - Physical connections of to-be-added leaves or servers should be established
    - The leaf doesn't attempt to add a new server to its topology view *even* if it receives a ping pkt from the DP
        - It's our responsiblity to add servers to leaves using the client APIs
    - In the leaf controller, the view of the *whole* topology isn't updated:
        - It *only* receives a refreshed list of its added/removed servers
        - It doesn't know the added/removed servers to/from other leaves
        - It doesn't know the other added/removed leaves
    - The same applies to the spine controller:
        - It *only* receives a refreshed list of its added/removed leaves and servers
        - It doesn't know the added/removed servers to/from other spines
        - It doesn't know the added/removed leaves to/from other spines
        - It doesn't know the other added/removed spines
    - Adding a new leaf controller could take ~5 seconds
        - This is because the `manager` needs to shutdown multiple components.

## Configurations
The configuration of the testbed should be described in a .toml file placed under /conf/ directory. 
The file describes available Switch ASICS, port configurations on the switches,  component details and the topology of the cluster (client IDs, switch IDs, servers and placement of workers in the racks).

Examples provided support the following configurations:
 1. Single-rack setup: Fig 6
 2. Multi-rack Uniform: Figs 7(a), 8(a), 9(a), 14(a), 15 (a)-(c), 16(a)-(c)
 3. Multi-rack Skewed: Figs 7(b), 8(b), 9(b), 14(b), 15 (d)-(f), 16 (d)-(f)

## Running Centralized and Switch controllers
Run centralized controller with an input config:
```
./bin/horus-central-ctrl ./conf/horus-topology.toml
```
Open another shell to each switch and run the switch manager:

```
./bin/horus-switch-mgr -topo ./conf/horus-topology.toml
```

## Collecting Stats
The switch controller reads the register data from dataplane from the ASIC to get overhead data. As control plane is orders of magnitude slower, these should be used to get values after a steady state and values in the middle of running experiment should not be relied on. We need to **stop the load generator first and after a few seconds stop the controller** to make sure the latest stats from the switch data plane is sampled by the controller. 

**Horus Controller Manager Output**
Upon exiting the horus-switch-mgr, it will automatically generate a JSON file in the format manager-stats-\<timestamp\>. 
The reported statistics includes:

- Total tasks arrived at leaf layer
- Total resubmitted tasks at leaf layer
- Total number of messages for load information
- Total number of messages for idle nodes information
- Total number of state update signals (Sum of load and idle information messages)
