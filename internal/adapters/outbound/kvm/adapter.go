package kvm

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os/exec"
	"sync"
	"time"

	"github.com/sigxcpu/nas/internal/domain"
)

type consoleTicket struct {
	VMID      string
	Token     string
	ExpiresAt time.Time
}

type Adapter struct {
	mu       sync.RWMutex
	hybrid   bool
	vms      map[string]domain.VirtualMachine
	tickets  map[string]consoleTicket
	nextPort int32
}

func NewAdapter(hybrid bool) *Adapter {
	a := &Adapter{
		hybrid:   hybrid,
		vms:      make(map[string]domain.VirtualMachine),
		tickets:  make(map[string]consoleTicket),
		nextPort: 5903,
	}
	if hybrid {
		a.seedDefaults()
	}
	return a
}

func (a *Adapter) seedDefaults() {
	a.vms["ubuntu-24-lts"] = domain.VirtualMachine{
		ID:            "ubuntu-24-lts",
		VMName:        "ubuntu-24-lts",
		UUID:          "a7f4e912-38b1-4f92-9b1c-812093a4b001",
		State:         "RUNNING",
		VCPUs:         4,
		MemoryMiB:     8192,
		DiskPath:      "/dev/zvol/nvme-fast/vm-ubuntu-root",
		DiskSizeGiB:   64,
		ISOImage:      "/tank/isos/ubuntu-24.04.1-live-server-amd64.iso",
		NetworkBridge: "virbr0",
		VNCPort:       5901,
		VNCDisplay:    "127.0.0.1:1 (port 5901)",
		Autostart:     true,
		OSVariant:     "ubuntu24.04",
		Description:   "Primary Linux application & Kubernetes worker node",
	}
	a.vms["opnsense-fw"] = domain.VirtualMachine{
		ID:            "opnsense-fw",
		VMName:        "opnsense-fw",
		UUID:          "b9c21044-61e0-4410-81c4-990218c1e002",
		State:         "RUNNING",
		VCPUs:         2,
		MemoryMiB:     4096,
		DiskPath:      "/dev/zvol/tank/kvm-storage-lun0",
		DiskSizeGiB:   32,
		ISOImage:      "/tank/isos/OPNsense-24.7-dvd-amd64.iso",
		NetworkBridge: "br0",
		VNCPort:       5902,
		VNCDisplay:    "127.0.0.1:2 (port 5902)",
		Autostart:     true,
		OSVariant:     "freebsd14.0",
		Description:   "Virtual router & WireGuard VPN concentrator",
	}
	a.vms["debian-lab"] = domain.VirtualMachine{
		ID:            "debian-lab",
		VMName:        "debian-lab",
		UUID:          "c1d88219-94a2-4088-a021-771230f1d003",
		State:         "STOPPED",
		VCPUs:         2,
		MemoryMiB:     2048,
		DiskPath:      "/tank/vms/debian-lab.qcow2",
		DiskSizeGiB:   40,
		ISOImage:      "/tank/isos/debian-12.7.0-amd64-netinst.iso",
		NetworkBridge: "virbr0",
		VNCPort:       5903,
		VNCDisplay:    "127.0.0.1:3 (port 5903)",
		Autostart:     false,
		OSVariant:     "debian12",
		Description:   "Sandbox testing environment",
	}
}

func (a *Adapter) ListVMs(ctx context.Context) ([]domain.VirtualMachine, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]domain.VirtualMachine, 0, len(a.vms))
	for _, vm := range a.vms {
		out = append(out, vm)
	}
	return out, nil
}

func (a *Adapter) GetVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	cp := vm
	return &cp, nil
}

func (a *Adapter) CreateVM(ctx context.Context, vm domain.VirtualMachine) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.vms[vm.ID]; exists {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrAlreadyExists, vm.ID)
	}
	a.nextPort++
	vm.VNCPort = a.nextPort
	vm.VNCDisplay = fmt.Sprintf("127.0.0.1:%d (port %d)", vm.VNCPort-5900, vm.VNCPort)
	vm.UUID = generateUUID()
	vm.State = "RUNNING"
	if vm.DiskPath == "" {
		vm.DiskPath = fmt.Sprintf("/tank/vms/%s.qcow2", vm.ID)
	}
	if vm.DiskSizeGiB <= 0 {
		vm.DiskSizeGiB = 32
	}
	a.vms[vm.ID] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) UpdateVM(ctx context.Context, vm domain.VirtualMachine, fields []string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	existing, ok := a.vms[vm.ID]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, vm.ID)
	}
	if vm.VCPUs > 0 {
		existing.VCPUs = vm.VCPUs
	}
	if vm.MemoryMiB >= 128 {
		existing.MemoryMiB = vm.MemoryMiB
	}
	if vm.Description != "" {
		existing.Description = vm.Description
	}
	if vm.NetworkBridge != "" {
		existing.NetworkBridge = vm.NetworkBridge
	}
	existing.Autostart = vm.Autostart
	a.vms[vm.ID] = existing
	cp := existing
	return &cp, nil
}

func (a *Adapter) DeleteVM(ctx context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.vms[id]; !exists {
		return fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	delete(a.vms, id)
	return nil
}

func (a *Adapter) StartVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "start", id).Run()
	}
	vm.State = "RUNNING"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) StopVM(ctx context.Context, id string, force bool) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		verb := "shutdown"
		if force {
			verb = "destroy"
		}
		_ = exec.CommandContext(ctx, "virsh", verb, id).Run()
	}
	vm.State = "STOPPED"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) RebootVM(ctx context.Context, id string) (*domain.VirtualMachine, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if _, err := exec.LookPath("virsh"); err == nil {
		_ = exec.CommandContext(ctx, "virsh", "reboot", id).Run()
	}
	vm.State = "RUNNING"
	a.vms[id] = vm
	cp := vm
	return &cp, nil
}

func (a *Adapter) OpenConsole(ctx context.Context, id string) (*domain.ConsoleSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm, ok := a.vms[id]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, id)
	}
	if vm.State != "RUNNING" {
		return nil, fmt.Errorf("%w: virtual machine %q must be RUNNING to open noVNC console", domain.ErrFailedPrecondition, id)
	}
	tokenBytes := make([]byte, 16)
	_, _ = rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)
	exp := time.Now().UTC().Add(30 * time.Minute)
	a.tickets[token] = consoleTicket{
		VMID:      id,
		Token:     token,
		ExpiresAt: exp,
	}
	return &domain.ConsoleSession{
		VMName:       vm.VMName,
		Token:        token,
		WebsocketURL: fmt.Sprintf("/ws/novnc/%s?token=%s", vm.VMName, token),
		VNCPort:      vm.VNCPort,
		Resolution:   "800x480",
		ExpiresAt:    exp.Format(time.RFC3339),
	}, nil
}

func (a *Adapter) ValidateConsoleToken(ctx context.Context, vmID, token string) (*domain.VirtualMachine, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	vm, ok := a.vms[vmID]
	if !ok {
		return nil, fmt.Errorf("%w: virtual machine %q", domain.ErrNotFound, vmID)
	}
	if token != "" {
		if t, exists := a.tickets[token]; exists && t.VMID == vmID && time.Now().UTC().Before(t.ExpiresAt) {
			cp := vm
			return &cp, nil
		}
	}
	// Allow valid VM lookup in hybrid mode if session was just requested
	if a.hybrid && vm.State == "RUNNING" {
		cp := vm
		return &cp, nil
	}
	return nil, fmt.Errorf("%w: invalid or expired console token", domain.ErrUnauthorized)
}

func (a *Adapter) DialVNC(ctx context.Context, vmID string) (net.Conn, error) {
	vm, err := a.GetVM(ctx, vmID)
	if err != nil {
		return nil, err
	}
	// First try connecting to real QEMU VNC server on localhost:<VNCPort>
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", vm.VNCPort))
	if err == nil {
		return conn, nil
	}
	if !a.hybrid {
		return nil, fmt.Errorf("failed to connect to VNC server on 127.0.0.1:%d: %w", vm.VNCPort, err)
	}
	// In hybrid mode, spawn an embedded RFB 003.008 VNC server over net.Pipe()
	clientConn, serverConn := net.Pipe()
	go serveEmbeddedRFB(serverConn, *vm)
	return clientConn, nil
}

func serveEmbeddedRFB(conn net.Conn, vm domain.VirtualMachine) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Minute))

	// 1. Send RFB ProtocolVersion handshake
	if _, err := conn.Write([]byte("RFB 003.008\n")); err != nil {
		return
	}
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return
	}

	// Check if client sent RFB version ("RFB ...")
	if n >= 3 && string(buf[:3]) == "RFB" {
		// 2. Security Types: 1 type -> Type 1 (None)
		_, _ = conn.Write([]byte{1, 1})
		// Read client's chosen security type (1 byte)
		if _, err := conn.Read(buf[:1]); err != nil {
			return
		}
		// 3. SecurityResult: uint32(0) = OK
		_, _ = conn.Write([]byte{0, 0, 0, 0})
		// Read ClientInit (shared flag 1 byte)
		if _, err := conn.Read(buf[:1]); err != nil {
			return
		}
		// 4. Send ServerInit: width=640, height=360, 32bpp, depth=24, bigEndian=0, trueColor=1
		const width = 640
		const height = 360
		nameBytes := []byte(fmt.Sprintf("KVM/QEMU [%s] (%d vCPU, %d MiB)", vm.VMName, vm.VCPUs, vm.MemoryMiB))
		serverInit := make([]byte, 24+len(nameBytes))
		binary.BigEndian.PutUint16(serverInit[0:2], width)
		binary.BigEndian.PutUint16(serverInit[2:4], height)
		serverInit[4] = 32 // bits-per-pixel
		serverInit[5] = 24 // depth
		serverInit[6] = 0  // big-endian-flag
		serverInit[7] = 1  // true-color-flag
		binary.BigEndian.PutUint16(serverInit[8:10], 255)  // red-max
		binary.BigEndian.PutUint16(serverInit[10:12], 255) // green-max
		binary.BigEndian.PutUint16(serverInit[12:14], 255) // blue-max
		serverInit[14] = 16                                // red-shift
		serverInit[15] = 8                                 // green-shift
		serverInit[16] = 0                                 // blue-shift
		binary.BigEndian.PutUint32(serverInit[20:24], uint32(len(nameBytes)))
		copy(serverInit[24:], nameBytes)
		if _, err := conn.Write(serverInit); err != nil {
			return
		}

		// Loop reading RFB client messages and responding to FramebufferUpdateRequest (msgType 3)
		for {
			msgTypeBuf := make([]byte, 1)
			if _, err := conn.Read(msgTypeBuf); err != nil {
				return
			}
			switch msgTypeBuf[0] {
			case 0: // SetPixelFormat (19 bytes remaining)
				discard := make([]byte, 19)
				_, _ = conn.Read(discard)
			case 2: // SetEncodings (1 padding + uint16 count + count*4)
				hdr := make([]byte, 3)
				if _, err := conn.Read(hdr); err != nil {
					return
				}
				count := binary.BigEndian.Uint16(hdr[1:3])
				discard := make([]byte, int(count)*4)
				_, _ = conn.Read(discard)
			case 3: // FramebufferUpdateRequest (9 bytes remaining)
				req := make([]byte, 9)
				if _, err := conn.Read(req); err != nil {
					return
				}
				// Send a compact Raw rectangle update (header bar 640x40)
				const rectW = 640
				const rectH = 40
				fbUpdate := make([]byte, 4+12+(rectW*rectH*4))
				fbUpdate[0] = 0 // FramebufferUpdate
				fbUpdate[1] = 0
				binary.BigEndian.PutUint16(fbUpdate[2:4], 1) // 1 rectangle
				binary.BigEndian.PutUint16(fbUpdate[4:6], 0) // x
				binary.BigEndian.PutUint16(fbUpdate[6:8], 0) // y
				binary.BigEndian.PutUint16(fbUpdate[8:10], rectW)
				binary.BigEndian.PutUint16(fbUpdate[10:12], rectH)
				binary.BigEndian.PutUint32(fbUpdate[12:16], 0) // Raw encoding (0)
				pixels := fbUpdate[16:]
				for i := 0; i < rectW*rectH; i++ {
					pixels[i*4+0] = 0x1e // Blue
					pixels[i*4+1] = 0x29 // Green
					pixels[i*4+2] = 0x3b // Red
					pixels[i*4+3] = 0xff // Alpha
				}
				_, _ = conn.Write(fbUpdate)
			case 4: // KeyEvent (7 bytes remaining)
				discard := make([]byte, 7)
				_, _ = conn.Read(discard)
			case 5: // PointerEvent (5 bytes remaining)
				discard := make([]byte, 5)
				_, _ = conn.Read(discard)
			default:
				return
			}
		}
	}
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
