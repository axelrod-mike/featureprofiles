// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aggregate_interface_counters_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/open-traffic-generator/snappi/gosnappi"
	"github.com/openconfig/featureprofiles/internal/attrs"
	"github.com/openconfig/featureprofiles/internal/deviations"
	"github.com/openconfig/featureprofiles/internal/fptest"
	"github.com/openconfig/featureprofiles/internal/otgutils"
	"github.com/openconfig/ondatra"
	"github.com/openconfig/ondatra/gnmi"
	"github.com/openconfig/ondatra/gnmi/oc"
	otgtelemetry "github.com/openconfig/ondatra/gnmi/otg"
	"github.com/openconfig/ondatra/netutil"
	"github.com/openconfig/ygnmi/ygnmi"
	"github.com/openconfig/ygot/ygot"
)

func TestMain(m *testing.M) {
	fptest.RunTests(m)
}

// Settings for configuring the aggregate testbed with the test
// topology.  IxNetwork flow requires both source and destination
// networks be configured on the ATE.  It is not possible to send
// packets to the ether.
//
// The testbed consists of ate:port1 -> dut:port1 and dut:port{2-4} ->
// ate:port{2-4}.  The first pair is called the "source" pair, and the
// second aggregate link the "destination" pair.
//
//   - Source: ate:port1 -> dut:port1 subnet 192.0.2.0/30 2001:db8::0/126
//   - Destination: dut:port{2-4} -> ate:port{2-4}
//     subnet 192.0.2.4/30 2001:db8::4/126
//
// A traffic flow is configured from ate:port1 as source and ate:port{2-9}
// as destination.
const (
	plen4 = 30
	plen6 = 126
)

var (
	dutSrc = attrs.Attributes{
		Desc:    "dutsrc",
		IPv4:    "192.0.2.1",
		IPv6:    "2001:db8::1",
		IPv4Len: plen4,
		IPv6Len: plen6,
	}

	ateSrc = attrs.Attributes{
		Name:    "atesrc",
		MAC:     "02:11:01:00:00:01",
		IPv4:    "192.0.2.2",
		IPv6:    "2001:db8::2",
		IPv4Len: plen4,
		IPv6Len: plen6,
	}

	dutDst = attrs.Attributes{
		Desc:    "dutdst",
		IPv4:    "192.0.2.5",
		IPv6:    "2001:db8::5",
		IPv4Len: plen4,
		IPv6Len: plen6,
	}

	ateDst = attrs.Attributes{
		Name:    "atedst",
		MAC:     "02:12:01:00:00:01",
		IPv4:    "192.0.2.6",
		IPv6:    "2001:db8::6",
		IPv4Len: plen4,
		IPv6Len: plen6,
	}
)

const (
	lagTypeLACP   = oc.IfAggregate_AggregationType_LACP
	lagTypeSTATIC = oc.IfAggregate_AggregationType_STATIC
)

type testCase struct {
	lagType oc.E_IfAggregate_AggregationType

	dut *ondatra.DUTDevice
	ate *ondatra.ATEDevice
	top gosnappi.Config

	// dutPorts is the set of ports the DUT -- the first (i.e., dutPorts[0])
	// is not configured in the aggregate interface.
	dutPorts []*ondatra.Port
	// atePorts is the set of ports on the ATE -- the first, as with the DUT
	// is not configured in the aggregate interface.
	atePorts  []*ondatra.Port
	aggID     string
	iteration int
}

func (tc *testCase) configSrcDUT(i *oc.Interface, a *attrs.Attributes) {
	i.Description = ygot.String(a.Desc)
	if deviations.InterfaceEnabled(tc.dut) {
		i.Enabled = ygot.Bool(true)
	}

	s := i.GetOrCreateSubinterface(0)
	s4 := s.GetOrCreateIpv4()
	if deviations.InterfaceEnabled(tc.dut) && !deviations.IPv4MissingEnabled(tc.dut) {
		s4.Enabled = ygot.Bool(true)
	}
	a4 := s4.GetOrCreateAddress(a.IPv4)
	a4.PrefixLength = ygot.Uint8(plen4)

	s6 := s.GetOrCreateIpv6()
	if deviations.InterfaceEnabled(tc.dut) {
		s6.Enabled = ygot.Bool(true)
	}
	s6.GetOrCreateAddress(a.IPv6).PrefixLength = ygot.Uint8(plen6)
}

func (tc *testCase) configDstAggregateDUT(i *oc.Interface, a *attrs.Attributes) {
	tc.configSrcDUT(i, a)
	i.Type = ieee8023adLag
	g := i.GetOrCreateAggregation()
	g.LagType = tc.lagType
}

func (tc *testCase) configDstMemberDUT(i *oc.Interface, p *ondatra.Port) {
	i.Description = ygot.String(p.String())
	i.Type = ethernetCsmacd

	if deviations.InterfaceEnabled(tc.dut) {
		i.Enabled = ygot.Bool(true)
	}

	e := i.GetOrCreateEthernet()
	e.AggregateId = ygot.String(tc.aggID)
}

func (tc *testCase) setupAggregateAtomically(t *testing.T) {
	d := &oc.Root{}

	if tc.lagType == lagTypeLACP {
		d.GetOrCreateLacp().GetOrCreateInterface(tc.aggID)
	}

	agg := d.GetOrCreateInterface(tc.aggID)
	agg.GetOrCreateAggregation().LagType = tc.lagType
	agg.Type = ieee8023adLag

	for _, port := range tc.dutPorts[1:] {
		i := d.GetOrCreateInterface(port.Name())
		i.GetOrCreateEthernet().AggregateId = ygot.String(tc.aggID)
		i.Type = ethernetCsmacd

		if deviations.InterfaceEnabled(tc.dut) {
			i.Enabled = ygot.Bool(true)
		}
	}

	p := gnmi.OC()
	fptest.LogQuery(t, fmt.Sprintf("%s to Update()", tc.dut), p.Config(), d)
	gnmi.Update(t, tc.dut, p.Config(), d)
}

func (tc *testCase) clearAggregate(t *testing.T) {
	d := gnmi.OC()

	for i := 0; i < 2; i++ {
		t.Logf("Delete/Recreate the aggregate on the device - Iteration %d", i)
		// Clear the aggregate minlink.
		t.Logf("Deleting the minlinks for the aggregate on the device")
		gnmi.Delete(t, tc.dut, d.Interface(tc.aggID).Aggregation().MinLinks().Config())

		// Clear the members of the aggregate.
		t.Logf("Deleting the members of the aggregate on the device")
		for _, port := range tc.dutPorts[1:] {
			gnmi.Delete(t, tc.dut, gnmi.OC().Interface(port.Name()).Ethernet().AggregateId().Config())
		}

		// Delete the aggregate device and re-create it.
		t.Logf("Deleting the aggregate on the device")
		agg := &oc.Interface{Name: ygot.String(tc.aggID)}
		agg.Type = ieee8023adLag
		gnmi.Delete(t, tc.dut, gnmi.OC().Interface(tc.aggID).Config())
		gnmi.Update(t, tc.dut, gnmi.OC().Interface(tc.aggID).Config(), agg)

		// Add the aggregate device and re-create it.
		tc.configDstAggregateDUT(agg, &dutDst)
		aggPath := d.Interface(tc.aggID)
		fptest.LogQuery(t, tc.aggID, aggPath.Config(), agg)
		gnmi.Replace(t, tc.dut, aggPath.Config(), agg)
		// Add the members of the aggregate.
		t.Logf("Adding the members of the aggregate on the device")
		for _, port := range tc.dutPorts[1:] {
			i := &oc.Interface{Name: ygot.String(port.Name())}
			i.Type = ethernetCsmacd
			gnmi.Update(t, tc.dut, gnmi.OC().Interface(port.Name()).Config(), i)
		}
		gnmi.Await(t, tc.dut, gnmi.OC().Interface(tc.aggID).Type().State(), time.Minute, ieee8023adLag)
	}
}

func (tc *testCase) configureDUT(t *testing.T) {
	t.Logf("dut ports = %v", tc.dutPorts)
	if len(tc.dutPorts) < 2 {
		t.Fatalf("Testbed requires at least 2 ports, got %d", len(tc.dutPorts))
	}

	d := gnmi.OC()

	if deviations.AggregateAtomicUpdate(tc.dut) {
		tc.clearAggregate(t)
		tc.setupAggregateAtomically(t)
	}

	// TODO - to remove this sleep later
	time.Sleep(5 * time.Second)

	agg := &oc.Interface{Name: ygot.String(tc.aggID)}
	tc.configDstAggregateDUT(agg, &dutDst)
	aggPath := d.Interface(tc.aggID)
	fptest.LogQuery(t, tc.aggID, aggPath.Config(), agg)
	gnmi.Replace(t, tc.dut, aggPath.Config(), agg)

	srcp := tc.dutPorts[0]
	srci := &oc.Interface{Name: ygot.String(srcp.Name())}
	tc.configSrcDUT(srci, &dutSrc)
	srci.Type = ethernetCsmacd
	srciPath := d.Interface(srcp.Name())
	fptest.LogQuery(t, srcp.String(), srciPath.Config(), srci)
	gnmi.Replace(t, tc.dut, srciPath.Config(), srci)
	if deviations.ExplicitInterfaceInDefaultVRF(tc.dut) {
		fptest.AssignToNetworkInstance(t, tc.dut, tc.aggID, deviations.DefaultNetworkInstance(tc.dut), 0)
		fptest.AssignToNetworkInstance(t, tc.dut, srcp.Name(), deviations.DefaultNetworkInstance(tc.dut), 0)
	}
	for _, port := range tc.dutPorts[1:] {
		i := &oc.Interface{Name: ygot.String(port.Name())}
		i.Type = ethernetCsmacd

		if deviations.InterfaceEnabled(tc.dut) {
			i.Enabled = ygot.Bool(true)
		}
		tc.configDstMemberDUT(i, port)
		iPath := d.Interface(port.Name())
		fptest.LogQuery(t, port.String(), iPath.Config(), i)
		gnmi.Replace(t, tc.dut, iPath.Config(), i)
	}
	if deviations.ExplicitPortSpeed(tc.dut) {
		for _, port := range tc.dutPorts {
			fptest.SetPortSpeed(t, port)
		}
	}
}

func (tc *testCase) configureATE(t *testing.T) {
	if len(tc.atePorts) < 2 {
		t.Fatalf("Testbed requires at least 2 ports, got: %v", tc.atePorts)
	}

	p0 := tc.atePorts[0]
	tc.top.Ports().Add().SetName(p0.ID())
	srcDev := tc.top.Devices().Add().SetName(ateSrc.Name)
	srcEth := srcDev.Ethernets().Add().SetName(ateSrc.Name + ".Eth").SetMac(ateSrc.MAC)
	srcEth.Connection().SetPortName(p0.ID())
	srcEth.Ipv4Addresses().Add().SetName(ateSrc.Name + ".IPv4").SetAddress(ateSrc.IPv4).SetGateway(dutSrc.IPv4).SetPrefix(uint32(ateSrc.IPv4Len))
	srcEth.Ipv6Addresses().Add().SetName(ateSrc.Name + ".IPv6").SetAddress(ateSrc.IPv6).SetGateway(dutSrc.IPv6).SetPrefix(uint32(ateSrc.IPv6Len))

	// Adding the rest of the ports to the configuration and to the LAG
	agg := tc.top.Lags().Add().SetName(ateDst.Name)
	if tc.lagType == lagTypeSTATIC {
		lagID, _ := strconv.Atoi(tc.aggID)
		agg.Protocol().Static().SetLagId(uint32(lagID))
		for i, p := range tc.atePorts[1:] {
			port := tc.top.Ports().Add().SetName(p.ID())
			newMac, err := incrementMAC(ateDst.MAC, i+1)
			if err != nil {
				t.Fatal(err)
			}
			agg.Ports().Add().SetPortName(port.Name()).Ethernet().SetMac(newMac).SetName("LAGRx-" + strconv.Itoa(i))
		}
	}

	// Disable FEC for 100G-FR ports because Novus does not support it.
	p100gbasefr := []string{}
	for _, p := range tc.atePorts {
		if p.PMD() == ondatra.PMD100GBASEFR {
			p100gbasefr = append(p100gbasefr, p.ID())
		}
	}

	if len(p100gbasefr) > 0 {
		l1Settings := tc.top.Layer1().Add().SetName("L1").SetPortNames(p100gbasefr)
		l1Settings.SetAutoNegotiate(true).SetIeeeMediaDefaults(false).SetSpeed("speed_100_gbps")
		autoNegotiate := l1Settings.AutoNegotiation()
		autoNegotiate.SetRsFec(false)
	}

	dstDev := tc.top.Devices().Add().SetName(agg.Name() + ".dev")
	dstEth := dstDev.Ethernets().Add().SetName(ateDst.Name + ".Eth").SetMac(ateDst.MAC)
	dstEth.Connection().SetLagName(agg.Name())
	dstEth.Ipv4Addresses().Add().SetName(ateDst.Name + ".IPv4").SetAddress(ateDst.IPv4).SetGateway(dutDst.IPv4).SetPrefix(uint32(ateDst.IPv4Len))
	dstEth.Ipv6Addresses().Add().SetName(ateDst.Name + ".IPv6").SetAddress(ateDst.IPv6).SetGateway(dutDst.IPv6).SetPrefix(uint32(ateDst.IPv6Len))

	// Fail early if the topology is bad.
	tc.ate.OTG().PushConfig(t, tc.top)
	tc.ate.OTG().StartProtocols(t)
}

const (
	ethernetCsmacd = oc.IETFInterfaces_InterfaceType_ethernetCsmacd
	ieee8023adLag  = oc.IETFInterfaces_InterfaceType_ieee8023adLag
	adminUp        = oc.Interface_AdminStatus_UP
	opUp           = oc.Interface_OperStatus_UP
	opDown         = oc.Interface_OperStatus_DOWN
	full           = oc.Ethernet_DuplexMode_FULL
	dynamic        = oc.IfIp_NeighborOrigin_DYNAMIC
)

func (tc *testCase) verifyAggID(t *testing.T, dp *ondatra.Port) {
	dip := gnmi.OC().Interface(dp.Name())
	di := gnmi.Get(t, tc.dut, dip.State())
	fptest.LogQuery(t, dp.String()+" before Await", dip.State(), di)

	if lagID := di.GetEthernet().GetAggregateId(); lagID != tc.aggID {
		t.Errorf("%s LagID got %v, want %v", dp, lagID, tc.aggID)
	}
}

func (tc *testCase) verifyInterfaceDUT(t *testing.T, dp *ondatra.Port) {
	dip := gnmi.OC().Interface(dp.Name())
	di := gnmi.Get(t, tc.dut, dip.State())
	fptest.LogQuery(t, dp.String()+" before Await", dip.State(), di)

	if got := di.GetAdminStatus(); got != adminUp {
		t.Errorf("%s admin-status got %v, want %v", dp, got, adminUp)
	}

	// LAG members may fall behind, so wait for them to be up.
	gnmi.Await(t, tc.dut, dip.OperStatus().State(), time.Minute, opUp)
}

func (tc *testCase) verifyDUT(t *testing.T) {
	// Wait for LAG negotiation and verify LAG type for the aggregate interface.
	gnmi.Await(t, tc.dut, gnmi.OC().Interface(tc.aggID).Type().State(), time.Minute, ieee8023adLag)

	for n, port := range tc.dutPorts {
		if n < 1 {
			// We designate port 0 as the source link, not part of LAG.
			t.Run(fmt.Sprintf("%s [source]", port.ID()), func(t *testing.T) {
				tc.verifyInterfaceDUT(t, port)
			})
			continue
		}
		t.Logf("Port Name: %s", port.ID())
		t.Run(fmt.Sprintf("%s [member]", port.ID()), func(t *testing.T) {
			tc.verifyInterfaceDUT(t, port)
			tc.verifyAggID(t, port)
		})
	}
}

// verifyATE checks the telemetry against the parameters set by
// configureDUT().
func (tc *testCase) verifyATE(t *testing.T) {
	ap := tc.atePorts[0]
	// State for the interface.
	time.Sleep(10 * time.Second)
	otgutils.LogLAGMetrics(t, tc.ate.OTG(), tc.top)

	if tc.lagType == oc.IfAggregate_AggregationType_LACP {
		otgutils.LogLACPMetrics(t, tc.ate.OTG(), tc.top)
	}
	portMetrics := gnmi.Get(t, tc.ate.OTG(), gnmi.OTG().Port(ap.ID()).State())
	if portMetrics.GetLink() != otgtelemetry.Port_Link_UP {
		t.Errorf("%s oper-status got %v, want %v", ap.ID(), portMetrics.GetLink(), otgtelemetry.Port_Link_UP)
	}
	t.Logf("Checking if LAG is up on OTG")
	gnmi.Watch(t, tc.ate.OTG(), gnmi.OTG().Lag(ateDst.Name).OperStatus().State(), time.Minute, func(val *ygnmi.Value[otgtelemetry.E_Lag_OperStatus]) bool {
		state, present := val.Val()
		return present && state.String() == "UP"
	}).Await(t)

}

// sortPorts sorts the ports by the testbed port ID.
func sortPorts(ports []*ondatra.Port) []*ondatra.Port {
	sort.SliceStable(ports, func(i, j int) bool {
		return ports[i].ID() < ports[j].ID()
	})
	return ports
}

// incrementMAC increments the MAC by i. Returns error if the mac cannot be parsed or overflows the mac address space
func incrementMAC(mac string, i int) (string, error) {
	macAddr, err := net.ParseMAC(mac)
	if err != nil {
		return "", err
	}
	convMac := binary.BigEndian.Uint64(append([]byte{0, 0}, macAddr...))
	convMac = convMac + uint64(i)
	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, convMac)
	if err != nil {
		return "", err
	}
	newMac := net.HardwareAddr(buf.Bytes()[2:8])
	return newMac.String(), nil
}

func (tc *testCase) TestAggregateCounters(t *testing.T) {
	dut := ondatra.DUT(t, "dut")
	gnmi.Await(t, dut, gnmi.OC().Interface(tc.aggID).Type().State(), time.Minute, ieee8023adLag)
	t.Logf("Waiting for interface to be up")
	t.Logf("Interface name: %s", tc.aggID)
	intn := gnmi.OC().Interface(tc.aggID).Name()
	t.Logf("Interface name: %v", intn)
	gnmi.Await(t, dut, gnmi.OC().Interface(tc.aggID).OperStatus().State(), time.Minute, opUp)
	intfCounters := gnmi.OC().Interface(tc.aggID).Counters()
	t.Logf("Interface counters: %v", intfCounters)
	subint := gnmi.OC().Interface(tc.aggID).Subinterface(0)
	ipv4Counters := subint.Ipv4().Counters()
	ipv6Counters := subint.Ipv6().Counters()
	intfCounterPath := "/interfaces/interface/state/counters/"
	ipv4CounterPath := "/interfaces/interface/subinterfaces/subinterface/ipv4/state/counters/"
	ipv6CounterPath := "/interfaces/interface/subinterfaces/subinterface/ipv6/state/counters/"

	skipSubinterfacePacketCountersMissing := deviations.SubinterfacePacketCountersMissing(dut)
	skipIpv6DiscardedPkts := skipSubinterfacePacketCountersMissing || deviations.Ipv6DiscardedPktsUnsupported(dut)

	cases := []struct {
		desc    string
		path    string
		counter ygnmi.SingletonQuery[uint64]
		skip    bool
	}{{
		desc:    "InUnicastPkts",
		path:    intfCounterPath + "in-unicast-pkts",
		counter: intfCounters.InUnicastPkts().State(),
	}, {
		desc:    "OutUnicastPkts",
		path:    intfCounterPath + "out-unicast-pkts",
		counter: intfCounters.OutUnicastPkts().State(),
	}, {
		desc:    "InMulticastPkts",
		path:    intfCounterPath + "in-multicast-pkts",
		counter: intfCounters.InMulticastPkts().State(),
	}, {
		desc:    "OutMulticastPkts",
		path:    intfCounterPath + "out-multicast-pkts",
		counter: intfCounters.OutMulticastPkts().State(),
	}, {
		desc:    "InPkts",
		path:    intfCounterPath + "in-pkts",
		counter: intfCounters.InPkts().State(),
	}, {
		desc:    "OutPkts",
		path:    intfCounterPath + "out-pkts",
		counter: intfCounters.OutPkts().State(),
	}, {
		desc:    "InDiscards",
		path:    intfCounterPath + "in-discards",
		counter: intfCounters.InDiscards().State(),
	}, {
		desc:    "OutDiscards",
		path:    intfCounterPath + "out-discards",
		counter: intfCounters.OutDiscards().State(),
	}, {
		desc:    "InErrors",
		path:    intfCounterPath + "in-errors",
		counter: intfCounters.InErrors().State(),
	}, {
		desc:    "OutErrors",
		path:    intfCounterPath + "out-errors",
		counter: intfCounters.OutErrors().State(),
	}, {
		desc:    "IPv4InPkts",
		path:    ipv4CounterPath + "in-pkts",
		counter: ipv4Counters.InPkts().State(),
	}, {
		desc:    "IPv4OutPkts",
		path:    ipv4CounterPath + "out-pkts",
		counter: ipv4Counters.OutPkts().State(),
	}, {
		desc:    "IPv6InPkts",
		path:    ipv6CounterPath + "in-pkts",
		counter: ipv6Counters.InPkts().State(),
	}, {
		desc:    "IPv6OutPkts",
		path:    ipv6CounterPath + "out-pkts",
		counter: ipv6Counters.OutPkts().State(),
	}, {
		desc:    "IPv6InDiscardedPkts",
		path:    ipv6CounterPath + "in-discarded-pkts",
		counter: ipv6Counters.InDiscardedPkts().State(),
		skip:    skipIpv6DiscardedPkts,
	}, {
		desc:    "IPv6OutDiscardedPkts",
		path:    ipv6CounterPath + "out-discarded-pkts",
		counter: ipv6Counters.OutDiscardedPkts().State(),
		skip:    skipIpv6DiscardedPkts,
	}}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.skip {
				t.Skipf("Counter %v is not supported.", tc.desc)
			}
			val, present := gnmi.Lookup(t, dut, tc.counter).Val()
			if !present {
				t.Errorf("Get IsPresent status for path %q: got false, want true", tc.path)
			}
			t.Logf("Got path/value: %s:%d", tc.path, val)
		})
	}
}

func TestAggregateCounters(t *testing.T) {
	dut := ondatra.DUT(t, "dut")
	ate := ondatra.ATE(t, "ate")
	aggID := netutil.NextAggregateInterface(t, dut)

	lagType := oc.IfAggregate_AggregationType_STATIC
	top := gosnappi.NewConfig()
	// Clean otg with an empty config
	ate.OTG().PushConfig(t, top)

	tc := &testCase{
		dut:     dut,
		ate:     ate,
		top:     top,
		lagType: lagType,

		dutPorts:  sortPorts(dut.Ports()),
		atePorts:  sortPorts(ate.Ports()),
		aggID:     aggID,
		iteration: 0,
	}

	t.Run(fmt.Sprintf("LagType=%s, Iteration=%d", lagType, 1), func(t *testing.T) {
		tc.configureDUT(t)
		time.Sleep(120 * time.Second)
		t.Run("VerifyDUT", tc.verifyDUT)
		tc.configureATE(t)
		t.Run("VerifyATE", tc.verifyATE)
		tc.TestAggregateCounters(t)
		t.Logf("Now clear the aggregate from device, re-create it and repeat the test")
		tc.clearAggregate(t)
	})
	t.Run(fmt.Sprintf("LagType=%s, Iteration=%d", lagType, 2), func(t *testing.T) {
		tc.configureDUT(t)
		time.Sleep(120 * time.Second)
		t.Run("VerifyDUT", tc.verifyDUT)
		t.Run("VerifyATE", tc.verifyATE)
		tc.TestAggregateCounters(t)
	})
}
