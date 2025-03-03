package virtio

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/vishvananda/netlink"
	"github.com/vm-tools/wtf/virtio/virtq"
	"golang.org/x/sys/unix"
)

const (
	netRxQ = 0
	netTxQ = 1
)

// Virtio Net Features
const (
	VirtioNetFCsum     = 1 << 0
	VirtioNetFMac      = 1 << 5
	VirtioNetFHostTso4 = 1 << 11
	VirtioNetFHostTso6 = 1 << 12
	VirtioNetFHostEcn  = 1 << 13
	VirtioNetFHostUfo  = 1 << 14
	VirtioNetFStatus   = 1 << 16
)

// VirtioNet Status Bits
const (
	VirtioNetLinkUp   = (1 << 0)
	VirtioNetAnnounce = (1 << 1)
)

// VirtioNet Config Space
const (
	VirtioNetMacOffset    = 0
	VirtioNetMacLen       = 6
	VirtioNetStatusOffset = (VirtioNetMacOffset + VirtioNetMacLen)
	VirtioNetStatusLen    = 2
	VirtioNetConfigLen    = (VirtioNetMacLen + VirtioNetStatusLen)
)

// VirtioNet VLAN support
const (
	VirtioNetHeaderSize = 10
)

// Header represents a VirtIO network device header (virtio_net_hdr)
type Header struct {
	Flags      uint8
	GSOType    uint8
	HdrLen     uint16
	GSOSize    uint16
	CSumStart  uint16
	CSumOffset uint16
	NumBuffers uint16 // not used in legacy drivers
}

const headerLength = 12

type NetDevice struct {
	// *VirtioDevice

	// The tap device file descriptor.
	Fd os.File `json:"fd"`

	// The mac address.
	Mac string `json:"mac"`

	// Size of vnet header expected by the tap device.
	Vnet int `json:"vnet"`

	// Hardware offloads supported by tap device?
	Offload bool `json:"offload"`
}

type netHandler struct {
	cfg NetDevice
	pw  *pcapgo.Writer
	f   *os.File
}

func (cfg NetDevice) NewHandler() (DeviceHandler, error) {
	f, _ := os.Create("/tmp/file.pcap")
	w := pcapgo.NewWriter(f)
	w.WriteFileHeader(65536, layers.LinkTypeEthernet)
	return &netHandler{pw: w, f: f, cfg: cfg}, nil
}

func (n *netHandler) GetType() DeviceID {
	return NetworkDeviceID
}

func (n *netHandler) GetFeatures() (features uint64) {

	return VirtioNetFCsum | VirtioNetFHostTso4 | VirtioNetFHostTso6 |
		VirtioNetFHostEcn | VirtioNetFHostUfo
}

func (n *netHandler) Close() error {
	n.f.Close()
	return nil
}

func (n *netHandler) QueueReady(num int, q *virtq.Queue, notify <-chan struct{}) error {
	switch num {
	case netRxQ:
		go func() {
			for range notify {
				if err := n.handleRx(q); err != nil {
					slog.Error("net tx: %v", "err", err)
				}
			}
		}()
	case netTxQ:
		go func() {
			for range notify {
				if err := n.handleTx(q); err != nil {
					slog.Error("net tx: %v", "err", err)
				}
			}
		}()
	}
	return nil
}
func (n *netHandler) Ready(negotiatedFeatures uint64) error {
	return nil
}

func (n *netHandler) ReadConfig(p []byte, off int) error {
	return nil
}

// From tap device
func (n *netHandler) handleRx(q *virtq.Queue) error {
	for {
		c, err := q.Next()
		if err != nil {
			return err
		}

		if c == nil {
			break
		}

		var bytesWritten int
		for i, d := range c.Desc {
			if !d.IsWO() {
				continue
			}

			buf, gbe := c.Buf(i)
			if gbe != nil {
				return gbe
			}

			//Strip the header bytesWritten, err = n.cfg.Fd.Read(buf[headerLength:])

			hdrFix := buf[10:]
			bytesWritten, err = n.cfg.Fd.Read(hdrFix)
			n.pw.WritePacket(gopacket.CaptureInfo{Timestamp: time.Now(), Length: len(hdrFix), CaptureLength: len(hdrFix), InterfaceIndex: 0}, hdrFix)

			break
		}

		if err != nil && err != io.EOF {
			return err
		}

		if err := c.Release(bytesWritten); err != nil {
			return err
		}
	}

	return nil
}

func (n *netHandler) handleTx(q *virtq.Queue) error {
	for {
		c, err := q.Next()
		if err != nil {
			return err
		}

		if c == nil {
			break
		}

		for i, d := range c.Desc {
			if d.IsWO() {
				break
			}

			buf, err := c.Buf(i)
			if err != nil {
				return err
			}

			hdrFix := append(make([]byte, 10), buf...)
			n.pw.WritePacket(gopacket.CaptureInfo{Timestamp: time.Now(), Length: len(hdrFix), CaptureLength: len(hdrFix), InterfaceIndex: 0}, hdrFix)
			if _, err := n.cfg.Fd.Write(hdrFix); err != nil {
				return err
			}
		}

		if err := c.Release(0); err != nil {
			return err
		}
	}

	return nil
}

// func (device *VirtioNetDevice) processPackets(
// 	vchannel *VirtioChannel,
// 	recv bool) error {

// 	for buf := range vchannel.incoming {

// 		header := buf.Map(0, VirtioNetHeaderSize)

// 		// Legit?
// 		if len(header) < VirtioNetHeaderSize {
// 			vchannel.outgoing <- buf
// 			continue
// 		}

// 		// Should we pass the virtio net header to the tap device as the vnet
// 		// header or strip it off?
// 		pktStart := VirtioNetHeaderSize - device.Vnet
// 		pktEnd := buf.Length() - pktStart

// 		// Doing send or recv?
// 		if recv {
// 			buf.Read(device.Fd, pktStart, pktEnd)
// 		} else {
// 			buf.Write(device.Fd, pktStart, pktEnd)
// 		}

// 		// Done.
// 		vchannel.outgoing <- buf
// 	}

// 	return nil
// }

// func NewVirtioMmioNet(info *DeviceInfo) (Device, error) {
// 	device, err := NewMmioVirtioDevice(info, VirtioTypeNet)
// 	device.Channels[0] = NewVirtioChannel(0, 256)
// 	device.Channels[1] = NewVirtioChannel(1, 256)
// 	return &VirtioNetDevice{VirtioDevice: device}, err
// }

// func NewVirtioPciNet(info *DeviceInfo) (Device, error) {
// 	device, err := NewPciVirtioDevice(info, PciClassNetwork, VirtioTypeNet, 16)
// 	device.Channels[0] = NewVirtioChannel(0, 256)
// 	device.Channels[1] = NewVirtioChannel(1, 256)
// 	return &VirtioNetDevice{VirtioDevice: device}, err
// }

// func (nic *VirtioNetDevice) Attach(vm *platform.Vm, model *Model) error {
// 	if nic.Vnet != 0 && nic.Vnet != VirtioNetHeaderSize {
// 		return VirtioUnsupportedVnetHeader
// 	}

// 	if nic.Vnet > 0 && nic.Offload {
// 		nic.Debug("hw offloads available, exposing features to guest.")
// 		nic.SetFeatures(VirtioNetFCsum | VirtioNetFHostTso4 | VirtioNetFHostTso6 |
// 			VirtioNetFHostEcn | VirtioNetFHostUfo)
// 	}

// 	// Set up our Config space.
// 	nic.Config.GrowTo(VirtioNetConfigLen)

// 	// Add MAC, if specified. If unspecified or bad
// 	// autogenerate.
// 	var mac net.HardwareAddr
// 	if nic.Mac != "" {
// 		var err error
// 		mac, err = net.ParseMAC(nic.Mac)
// 		if err != nil {
// 			return err
// 		}
// 	} else {
// 		// Random MAC with Gridcentric's OUI.
// 		mac = make([]byte, 6)
// 		rand.Read(mac[3:])
// 		mac[0] = 0x28
// 		mac[1] = 0x48
// 		mac[2] = 0x46
// 	}
// 	nic.SetFeatures(VirtioNetFMac)
// 	for i := 0; i < len(mac); i += 1 {
// 		nic.Config.Set8(VirtioNetMacOffset+i, mac[i])
// 	}

// 	// Add status bits. In the future we should
// 	// be polling the underlying physical/tap device
// 	// for link-up and announce status. For now,
// 	// just emulate the status-less "always up" behavior.
// 	nic.SetFeatures(VirtioNetFStatus)
// 	nic.Config.Set16(VirtioNetStatusOffset, VirtioNetLinkUp)

// 	err := nic.VirtioDevice.Attach(vm, model)
// 	if err != nil {
// 		return err
// 	}

// 	// Start our network process.
// 	go nic.processPackets(nic.Channels[0], true)
// 	go nic.processPackets(nic.Channels[1], false)

// 	return nil
// }

func CreateTap(name string, mtu int, ownerUID, ownerGID int) (*netlink.Tuntap, error) {

	// set reasonable defaults (perhaps)
	if mtu == 0 {
		mtu = 1500
	}
	if ownerGID == 0 {
		ownerGID = os.Getgid()
	}
	if ownerUID == 0 {
		ownerUID = os.Getuid()
	}

	tapLinkAttrs := netlink.NewLinkAttrs()
	tapLinkAttrs.Name = name
	tapLink := &netlink.Tuntap{
		LinkAttrs: tapLinkAttrs,

		// We want a tap device (L2) as opposed to a tun (L3)
		Mode: netlink.TUNTAP_MODE_TAP,

		// Firecracker does not support multiqueue tap devices at this time:
		// https://github.com/firecracker-microvm/firecracker/issues/750
		Queues: 1,

		Flags: netlink.TUNTAP_ONE_QUEUE | // single queue tap device
			netlink.TUNTAP_VNET_HDR, // parse vnet headers added by the vm's virtio_net implementation
	}

	err := netlink.LinkAdd(tapLink)
	if err != nil {
		return nil, fmt.Errorf("failed to create tap device: %w", err)
	}

	for _, tapFd := range tapLink.Fds {
		err = unix.IoctlSetInt(int(tapFd.Fd()), unix.TUNSETOWNER, ownerUID)
		if err != nil {
			return nil, fmt.Errorf("failed to set tap %s owner to uid %d: %w", name, ownerUID, err)
		}

		err = unix.IoctlSetInt(int(tapFd.Fd()), unix.TUNSETGROUP, ownerGID)
		if err != nil {
			return nil, fmt.Errorf("failed to set tap %s group to gid %d: %w", name, ownerGID, err)
		}
	}

	err = netlink.LinkSetMTU(tapLink, mtu)
	if err != nil {
		return nil, fmt.Errorf("failed to set tap device MTU to %d: %w", mtu, err)
	}

	err = netlink.LinkSetUp(tapLink)
	if err != nil {
		return nil, errors.New("failed to set tap up")
	}

	return tapLink, nil
}
