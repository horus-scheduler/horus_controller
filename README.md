# Horus Control Plane

This repository contains our implementation of the control plane of the Horus in-network task scheduler. 

Horus has both data plane and control plane components. 
The control plane of Horus consists of a centralized controller, spine controllers, and leaf controllers. While the data plane schedules tasks, the control plane handles initialization, failures, and dynamic virtual clusters.


## Design


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

1. **Design-specific Assumptions**
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

## Todo
- [ ] 
- [ ] 
- [ ]