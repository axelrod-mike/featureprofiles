
# DP-1.17: Egress traffic DSCP rewrite
## Summary

This test validates egress traffic scheduling and packet remarking (rewrite) on a DUT. IP payload encapsulated as IPoGRE,
IPoMPLSoGRE,IPoGUE or IPoMPLSoGUE would be decapsulated by DUT as per tunnel termination configuration and forwarded as an 
IP payload via the egress interface. IP payload should be scheduled via egress interface according to DSCP markings prior to rewrite.
DSCP values of IP payload encapsulated as IPoGRE, IPoMPLSoGRE, IPoGUE or IPoMPLSoGUE should have payload DSCP values re-written 
per egress QOS re-write policy applied to the egress interface. Egress QOS policy should support match conditions on payload 
IP header fields, specifically DSCP values. Based on the match condition, DSCP must be set to a new value. DUT configuration 
will be evaluated against OpenConfig QOS model, and traffic flows analyzed to ensure proper scheduling, re-marking, and forwarding.

## Testbed type

* [`featureprofiles/topologies/atedut_2.testbed`](https://github.com/openconfig/featureprofiles/blob/main/topologies/atedut_2.testbed)

## Procedure

### Test environment setup

* DUT has an ingress port and 1 egress port.

| | [ ATE Port 1 ] ----- | DUT | ----- [ ATE Port 2 ] | |


* Configure DUT's ingress and egress interfaces.


#### Configuration

1. DUT:Port1 is a Singleton IP interface towards ATE:Port1.
2. DUT:Port2 is a Singleton IP interface towards ATE:Port2.
3. DUT forms one IPv4 and one IPV6 eBGP session with ATE:Port1 using connected Singleton interface IPs.
4. DUT forms one IPv4 and one IPV6 eBGP session with ATE:Port2 using connected Singleton interface IPs.
5. DUT has IPv4-DST-DECAP/32 and IPv6-DST-DECAP/128 advertised to ATE:Port1 via IPv4 BGP. This IP is used for decapsulation.
6. ATE:Port2 advertises destination networks IPv4-DST-NET/32 and IPv6-DST-NET/128 to DUT.
7. DUT decapsulates IPoGRE, IPoMPLSoGRE,IPoGUE or IPoMPLSoGUE payload with destination of IPv4-DST-DECAP/32 and IPv6-DST-DECAP/128
8. DUT has MPLS static forwarding rule for LabelEntry matching outer label 100 pointing to a NHG resolved via ATE:Port2.
9. DUT matches packets on DSCP/TC values and sets new DSCP values based on remarking rules.


* Re-marking table

Forwarding Group	IPv4 TOS	IPv6 TC
be1	0	0
af1	0	0
af2	0	0
af3	0	0
af4	0	0
nc1	6	48

### DP-1.17.1 Egress Classification and rewrite of IPv4 packets with various DSCP values

* Traffic:
    * Generate IPv4 traffic from ATE Port 1 with various DSCP values.
* Verification:
    * Monitor telemetry on DUT to verify packet scheduling into correct forwarding groups.
    * Capture packets on ATE's ingress to verify packet re-marking per table.
    * Analyze traffic flows to confirm no packet drops on DUT.

### DP-1.17.2 Egress Classification and rewrite of IPv6 packets with various TC values

* Traffic:
    * Generate IPv6 traffic from ATE Port 1 with various TC values
* Verification:
    * Monitor telemetry on DUT to verify packet scheduling into correct forwarding groups.
    * Capture packets on ATE's ingress to verify packet marking per table.
    * Analyze traffic flows, confirming no drops on DUT.

### DP-1.17.3 Egress Classification and rewrite of IPoMPLSoGUE traffic with pop action
Member
@dplore dplore last week
I think the IPoMPLSoGUE and IPoMPLSoGRE subtests here are possibly duplicates of this test:
https://github.com/openconfig/featureprofiles/tree/ac98ad2d2a57b460ab6584d4b26f99145a04ba7e/feature/policy_forwarding/otg_tests/mpls_gre_udp_qos

Can you verify that PF-1.18 mpls_gre_udp_qos is a duplicate of those scenarios? IF yes, you can remove the duplicated subtests from this PR.

@axelrod-mike	Reply...

*   Configuration:
    *   Configure Static MPLS LSP with MPLS pop and IPv4/IPv6 forward actions for a specific labels range (100020-100100).
    *   Configure decapsulation rules for IPv4-DST-DECAP/32
*   Traffic:
    *   Generate IPoMPLSoGUE traffic from ATE Port 1 with labels between 100020 and 1000100
*   Verfication:
    *   Monitor telemetry on the DUT to verify that packets re being scehduled for transmission into correct forwarding groups.
    *   Capture packets on the ATE's ingress interface to verify packet marking according to the marking table.
    *   Analyze traffic flows to confirm that no packets are dropped on the DUT.

### DP-1.17.4 Egress Classification and rewrite of IPv6oMPLSoGUE traffic with pop action
*   Configuration:
    *   Configure Static MPLS LSP with MPLS pop and IPv6 forward actions for a specific labels range (100020-100100).
    *    Configure decapsulation rules for IPv6-DST-DECAP/12 
*   Traffic:
    *   Generate IPv6oMPLSoGUE traffic from ATE Port 1 with labels between 100020 and 1000100
*   Verfication:
    *   Monitor telemetry on the DUT to verify that packets re being scehduled for transmission into correct forwarding groups.
    *   Capture packets on the ATE's ingress interface to verify packet marking according to the marking table.
    *   Analyze traffic flows to confirm that no packets are dropped on the DUT.

### DP-1.17.5 Egress Classification and rewrite of IPoMPLSoGRE traffic with pop action
*   Configuration:
    *   Configure Static MPLS LSP with MPLS pop and IPv4 forward actions for a specific labels range (100020-100100).
    *   Configure decapsulation rules for IPv4-DST-DECAP/32
*   Traffic:
    *   Generate IPoMPLSoGRE traffic from ATE Port 1 with labels between 100020 and 1000100
*   Verfication:
    *   Monitor telemetry on the DUT to verify that packets re being scehduled for transmission into correct forwarding groups.
    *   Capture packets on the ATE's ingress interface to verify packet marking according to the marking table.
    *   Analyze traffic flows to confirm that no packets are dropped on the DUT.

### DP-1.17.7 Egress Classification and rewrite of IPv6oMPLSoGRE traffic with pop action
*   Configuration:
    *   Configure Static MPLS LSP with MPLS pop and IPv6 forward actions for a specific labels range (100020-100100).
    *    Configure decapsulation rules for IPv6-DST-DECAP/12 
*   Traffic:
    *   Generate IPv6oMPLSoGRE traffic from ATE Port 1 with labels between 100020 and 1000100
*   Verfication:
    *   Monitor telemetry on the DUT to verify that packets re being scehduled for transmission into correct forwarding groups.
    *   Capture packets on the ATE's ingress interface to verify packet marking according to the marking table.
    *   Analyze traffic flows to confirm that no packets are dropped on the DUT.

### DP-1.17.8 Egress Classification and rewrite of IPoGRE traffic with pop action
*   Configuration:
    *    Configure decapsulation rules for IPv4-DST-DECAP/32
*   Traffic:
    *   Generate IPoGRE traffic from ATE Port 1 with IP payload dest reachable via ATE:Port2
*   Verfication:
    *   Monitor telemetry on the DUT to verify that packets re being scehduled for transmission into correct forwarding groups.
    *   Capture packets on the ATE's ingress interface to verify packet marking according to the marking table.
    *   Analyze traffic flows to confirm that no packets are dropped on the DUT.

### DP-1.17.9 Egress Classification and rewrite of IPv6oGRE traffic with pop action
*   Configuration:
    *   Configure decapsulation rules for  IPv6-DST-DECAP/12 
*   Traffic:
    *   Generate IPv6oGRE traffic from ATE Port 1 with payload reachable via ATE:Port2
*   Verfication:
    *   Monitor telemetry on the DUT to verify that packets re being scehduled for transmission into correct forwarding groups.
    *   Capture packets on the ATE's ingress interface to verify packet marking according to the marking table.
    *   Analyze traffic flows to confirm that no packets are dropped on the DUT.
Comment on lines +115 to +133
Member
@dplore dplore last week
Please review PF-1.3 Policy-based IPv4 GRE Decapsulation

I think this test overlaps with PF-1.3, but adds some packet marking rules. In order to specify exactly what configuration is needed (add structure to expose how this qos test is related to the OC policy-forwarding test for GRE decap), please add a link to PF-1.3 for the required IPoGRE decap configuration rules.

@axelrod-mike	Reply...

### DP-1.17.10 Egress Classification and rewrite of IPoGUE traffic with pop action
*   Configuration:
    *    Configure decapsulation rules for IPv4-DST-DECAP/32
*   Traffic:
    *   Generate IPoGUE traffic from ATE Port 1 with payload reachable via ATE:Port2
*   Verfication:
    *   Monitor telemetry on the DUT to verify that packets re being scehduled for transmission into correct forwarding groups.
    *   Capture packets on the ATE's ingress interface to verify packet marking according to the marking table.
    *   Analyze traffic flows to confirm that no packets are dropped on the DUT.

### DP-1.17.11 Egress Classification and rewrite of IPv6oGUE traffic with pop action
*   Configuration:
    *   Configure decapsulation rules for Id IPv6-DST-DECAP/12 
*   Traffic:
    *   Generate IPv6oGUE traffic from ATE Port 1 with IPv6 payload reachable via ATE:Port2
*   Verfication:
    *   Monitor telemetry on the DUT to verify that packets re being scehduled for transmission into correct forwarding groups.
    *   Capture packets on the ATE's ingress interface to verify packet marking according to the marking table.
    *   Analyze traffic flows to confirm that no packets are dropped on the DUT.


## OpenConfig Path and RPC Coverage

The below yaml defines the OC paths intended to be covered by this test.

```yaml
paths:
  ## Config paths
  /qos/classifiers/classifier/config/name:
  /qos/classifiers/classifier/config/type:
  /qos/classifiers/classifier/terms/term/config/id:
  /qos/classifiers/classifier/terms/term/actions/config/target-group:
  /qos/classifiers/classifier/terms/term/conditions/ipv4/config/dscp:
  /qos/classifiers/classifier/terms/term/conditions/ipv4/config/dscp-set:
  /qos/classifiers/classifier/terms/term/conditions/ipv6/config/dscp:
  /qos/classifiers/classifier/terms/term/conditions/ipv6/config/dscp-set:
  /qos/classifiers/classifier/terms/term/conditions/mpls/config/traffic-class:
  /qos/classifiers/classifier/terms/term/actions/remark/config/set-dscp:
  /qos/classifiers/classifier/terms/term/actions/remark/config/set-mpls-tc:
  /qos/interfaces/interface/input/classifiers/classifier/config/name:
  /qos/interfaces/interface/input/classifiers/classifier/config/type:

  ## State paths
  /qos/interfaces/interface/input/classifiers/classifier/terms/term/state/matched-packets:
  /qos/interfaces/interface/input/classifiers/classifier/terms/term/state/matched-octets:

rpcs:
  gnmi:
    gNMI.Set:
    gNMI.Subscribe:
```

## Minimum DUT platform requirement

* FFF - fixed form factor
