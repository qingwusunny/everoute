package datapath

import (
	"fmt"
	"net"

	"github.com/contiv/ofnet/ofctrl"
	openflow "github.com/contiv/libOpenflow/openflow13"
)

var (
	LBOInputTable          uint8 = 0
	LBOArpProxyTable       uint8 = 10
	LBOInPortTable         uint8 = 30
	LBOFromNatTable        uint8 = 40
	LBOFromPolicyTable     uint8 = 50
	LBOFromLocalTable      uint8 = 60
	LBOForwardToLocalTable uint8 = 80
	LBOPaddingL2Table      uint8 = 90
	LBOOutputTable         uint8 = 110
)

type LocalBridgeOverlay struct {
	BaseBridge

	inputTable          *ofctrl.Table
	arpProxyTable       *ofctrl.Table
	inPortTable         *ofctrl.Table
	fromNatTable        *ofctrl.Table
	fromPolicyTable     *ofctrl.Table
	fromLocalTable      *ofctrl.Table
	forwardToLocalTable *ofctrl.Table
	paddingl2Table      *ofctrl.Table
	outputTable         *ofctrl.Table
}

func NewLocalBridgeOverlay(brName string, datapathManager *DpManager) *LocalBridgeOverlay {
	localBridge := &LocalBridgeOverlay{}
	localBridge.name = brName
	localBridge.datapathManager = datapathManager

	return localBridge
}

func (l *LocalBridgeOverlay) BridgeInitCNI() {
	if !l.datapathManager.IsEnableOverlay() {
		return
	}

	sw := l.OfSwitch

	l.inputTable = sw.DefaultTable()
	l.arpProxyTable, _ = sw.NewTable(LBOArpProxyTable)
	l.inPortTable, _ = sw.NewTable(LBOInPortTable)
	l.fromNatTable, _ = sw.NewTable(LBOFromNatTable)
	l.fromPolicyTable, _ = sw.NewTable(LBOFromPolicyTable)
	l.fromLocalTable, _ = sw.NewTable(LBOFromLocalTable)
	l.forwardToLocalTable, _ = sw.NewTable(LBOForwardToLocalTable)
	l.paddingl2Table, _ = sw.NewTable(LBOPaddingL2Table)
	l.outputTable, _ = sw.NewTable(LBOOutputTable)

}

func (l *LocalBridgeOverlay) initInputTable(sw *ofctrl.OFSwitch) error {
	arpFlow, _ := l.inputTable.NewFlow(ofctrl.FlowMatch{
		Ethertype: PROTOCOL_ARP,
		Priority:  NORMAL_MATCH_FLOW_PRIORITY,
	})
	if err := arpFlow.Resubmit(nil, &LBOArpProxyTable); err != nil {
		return fmt.Errorf("failed to setup input table arp flow resubmit to arp proxy table action, err: %v", err)
	}
	if err := arpFlow.Next(ofctrl.NewEmptyElem()); err != nil {
		return fmt.Errorf("faile to install input table arp flow, err: %v", err)
	}

	ipFlow, _ := l.inputTable.NewFlow(ofctrl.FlowMatch{
		Ethertype: PROTOCOL_IP,
		Priority:  NORMAL_MATCH_FLOW_PRIORITY,
	})
	if err := ipFlow.Resubmit(nil, &LBOInPortTable); err != nil {
		return fmt.Errorf("failed to setup input table ip flow resubmit to in port table action, err: %v", err)
	}
	if err := ipFlow.Next(ofctrl.NewEmptyElem()); err != nil {
		return fmt.Errorf("failed to install input table ip flow, err: %v", err)
	}

	defaultFlow, _ := l.inputTable.NewFlow(ofctrl.FlowMatch{
		Priority: DEFAULT_FLOW_MISS_PRIORITY,
	})
	if err := defaultFlow.Next(sw.DropAction()); err != nil {
		return fmt.Errorf("failed to install input table default flow, err: %v", err)
	}

	return nil
}

func (l *LocalBridgeOverlay) initArpProxytable(sw *ofctrl.OFSwitch) error {
	arpProxyFlow, _ := l.arpProxyTable.NewFlow(ofctrl.FlowMatch{
		Ethertype: PROTOCOL_ARP,
		ArpTpa: &l.datapathManager.Info.ClusterPodCidr.IP,
		ArpTpaMask: (*net.IP)(&l.datapathManager.Info.ClusterPodCidr.Mask),
		Priority: MID_MATCH_FLOW_PRIORITY,
	})
	if err := arpProxyFlow.MoveField(MacLength, 0, 0, "nxm_of_eth_src", "nxm_of_eth_dst", false); err != nil {
		return err
	}
	if err := arpProxyFlow.SetMacSa(net.HardwareAddr(FACK_MAC)); err != nil {
		return err
	}
	if err := arpProxyFlow.LoadField("nxm_of_arp_op", ArpOperReply, openflow.NewNXRange(0, 15)); err != nil {
		return err
	}
}

