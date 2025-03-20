# WTF

Don't create a VMM yourself, it's madness.

## Features

- Pure Go implementation (minimal CGo usage)
- Linux kernel (bzImage) and initrd support
- VirtIO devices:
  - Console (terminal I/O)
  - Network (TAP interface)
  - Block devices (file, HTTP, memory storage)
  - Socket support
- Configurable VM memory size
- Raw terminal handling

## Prerequisites

- Linux system with KVM support
- Go 1.21 or later
- Root privileges (for TAP interface creation)

## Installation

```bash
git clone https://github.com/kube-vm-project/wtf.git
cd wtf
make
```

## Usage

Basic usage with a kernel image:
```bash
sudo ./wtf -kernel /path/to/bzImage
```

### Command Line Options

- `-mem SIZE`: Set VM memory size in MiB (default: 1024)
- `-kernel PATH`: Load kernel image from file or URL (default: "bzImage")
- `-initrd PATH`: Load initial ramdisk from file or URL
- `-cmdline STRING`: Set kernel command line (default: "console=hvc0 reboot=t")
- `-tapname NAME`: Create and attach a TAP network interface
- `-block DEVICE`: Add a block device (can be specified multiple times)

### Block Device Examples

```bash
# Read-write file-based storage
./wtf -block file:///path/to/disk.img

# Read-only file-based storage
./wtf -block file:///path/to/disk.img:ro

# HTTP-based storage (always read-only)
./wtf -block http://example.com/disk.img

# Memory-based storage (size in bytes)
./wtf -block mem://1048576
```

## Project Structure

- `/kvm/`: KVM syscall interfaces and low-level functionality
- `/virtio/`: VirtIO device implementations
- `/vmm/`: Virtual machine management code
- `/os/linux/`: Linux-specific boot and loader code
- `/cmd/`: Additional utility commands

## Contributing

Please feel free to submit issues and pull requests.
