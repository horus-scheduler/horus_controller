# mDC Controller

## Overview
This repository contains our implementation of the control plane of multicast for data center networks (mDC).
mDC relies on a *hierarchical* algorithm for state management and failure recovery (more in the details). 
That is, one centralized controller and distributed controllers (at ToR switches) agree on how to maintain state and recover from failures of the data plane. 

A data center network, running mDC, would deploy one process, called `mdc-central-ctrl`, as the centralized controller of the network. 
In addition, in a network of N edge routers, the network deploys N `mdc-switch-ctrl` processes. Each of runs as the control plane of the corresponding edge switch (i.e., ToR switch). 
Currently, `mdc-switch-ctrl` is built entirely in software.

## The Design
### Centralized Controller

### ToR Switch Controller

## mDC Packet Layout

### Data Packet
|      |    |         |
|------|----|---------|


## Building

Dependencies:
- golang 1.12

Build both `mdc-central-ctrl` and `mdc-switch-ctrl`:

    make all




## TODO
- [ ] Can we implement the ToR controller in hardware?
- [ ] Handling failures of the ToR
- [ ] Study the effect of incorrect forwarding during changing the state
    - [ ] Use switch memory during the transition time
