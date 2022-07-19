# https://github.com/barefootnetworks/Open-Tofino/blob/master/include/bf_types/bf_types.h
FEC_MAP = {
    'NONE': 'BF_FEC_TYP_NONE',
    'FC': 'BF_FEC_TYP_FIRECODE',
    'RS': 'BF_FEC_TYP_REED_SOLOMON',
}

# https://github.com/barefootnetworks/Open-Tofino/blob/master/include/bf_pm/bf_pm_intf.h
AN_MAP = {
    0: 'PM_AN_DEFAULT',
    1: 'PM_AN_FORCE_ENABLE',
    2: 'PM_AN_FORCE_DISABLE',
}

# Fill the `links_by_cages` set of tuples as follow:
# A tuple of the shape: (<cage/lane>, <speed>, <FEC>, <AN_MODE>)
# CAGE/LANE examples: '1/-', '5/0'
# SPEED = 1G | 10G | 25G | 40G | 50G | 100G
# FEC = NONE | FC (firecode for 40G) | RS (Reed-Solomon for 100G)
# AN_MODE = 0 (determined by SDE) | 1 (on) | 2 (off)

# IMPORTNT NOTE: it's your responsibility to ensure that the cage/lane and speed values are consistent!

links_by_cages = {
    ('1/-', '10G', 'NONE', 2),
    ('2/-', '10G', 'NONE', 2),
    ('3/-', '10G', 'NONE', 2),
    ('4/-', '10G', 'NONE', 2),
    ('9/-', '10G', 'NONE', 2),
    ('19/0', '100G', 'NONE', 0),
    ('20/0', '100G', 'NONE', 0),
    ('21/0', '100G', 'NONE', 0),
    ('22/0', '100G', 'NONE', 0),
    ('23/0', '100G', 'NONE', 0),
    ('24/0', '100G', 'NONE', 0),
    ('25/0', '100G', 'NONE', 0),
    ('26/0', '100G', 'NONE', 0),
    ('27/0', '100G', 'NONE', 0),
    ('28/0', '100G', 'NONE', 0),
    ('29/0', '100G', 'NONE', 0),
    ('30/0', '100G', 'NONE', 0),
    }

links_by_dp = set()

for port_tuple in links_by_cages:
    if len(port_tuple) != 4:
        continue
    if '/' in port_tuple[0]:
        port_spec = port_tuple[0].split('/')
        if len(port_spec) == 2:
            cage = int(port_spec[0])
            if port_spec[1] == '-':
                for lane in [0, 1, 2, 3]:
                    dp = bfrt.port.port_hdl_info.get(CONN_ID=cage, CHNL_ID=lane, print_ents=False).data[b'$DEV_PORT']
                    links_by_dp.add((dp, port_tuple[1], port_tuple[2], port_tuple[3]))
            else:
                lane = int(port_spec[1])
                dp = bfrt.port.port_hdl_info.get(CONN_ID=cage, CHNL_ID=lane, print_ents=False).data[b'$DEV_PORT']
                links_by_dp.add((dp, port_tuple[1], port_tuple[2], port_tuple[3]))

for dp_tuple in links_by_dp:
    dp = dp_tuple[0]
    speed = dp_tuple[1]
    fec = dp_tuple[2]
    an = dp_tuple[3]
    
    bf_speed = ''
    bf_fec = ''
    bf_an = ''

    if speed in ['1G', '10G', '25G', '40G', '50G', '100G']:
        bf_speed = 'BF_SPEED_' + speed
    if fec in FEC_MAP:
        bf_fec = FEC_MAP[fec]
    if an in AN_MAP:
        bf_an = AN_MAP[an]
    
    if bf_speed != '' and bf_fec != '' and bf_an != '':
        bfrt.port.port.add(DEV_PORT=dp, 
        SPEED=bf_speed, 
        FEC=bf_fec, 
        AUTO_NEGOTIATION=bf_an, 
        PORT_ENABLE=True)

