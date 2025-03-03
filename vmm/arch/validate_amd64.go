package arch

import "github.com/vm-tools/wtf/kvm"

var archCaps = []kvm.Cap{
	kvm.CapExtCPUID,
	kvm.CapTSCDeadlineTimer,
}
