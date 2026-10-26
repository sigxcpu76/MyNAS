package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/sigxcpu/nas/internal/domain"
	nasv1 "github.com/sigxcpu/nas/internal/gen/nas/v1"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func newPrereqCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prereq",
		Short: "Inspect and verify host prerequisites",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all system prerequisites and their status",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.ListPrerequisites(cmd.Context(), &nasv1.ListPrerequisitesRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetPrerequisites())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tCATEGORY\tKIND\tSTATUS\tVERSION\tTARGET")
			for _, p := range resp.GetPrerequisites() {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					p.GetName(), p.GetCategory(), p.GetKind(), p.GetStatus(), p.GetVersion(), p.GetBinaryOrPath())
			}
			return tw.Flush()
		},
	}


	printReport := func(resp *nasv1.CheckPrerequisitesResponse) error {
		if rootOpts.JSONOutput {
			return rootOpts.printJSON(resp)
		}

		flavor := resp.GetLinuxFlavor()
		flavorName := "Unknown Linux"
		pkgMgr := "unknown"
		if flavor != nil {
			if flavor.GetPrettyName() != "" {
				flavorName = flavor.GetPrettyName()
			} else if flavor.GetName() != "" {
				flavorName = flavor.GetName()
			}
			if flavor.GetPackageManager() != "" {
				pkgMgr = flavor.GetPackageManager()
			}
		}

		fmt.Fprintln(rootOpts.Out, "================================================================================")
		fmt.Fprintln(rootOpts.Out, "PREREQUISITES STATUS REPORT")
		fmt.Fprintf(rootOpts.Out, "Host Linux Flavor: %s (package manager: %s)\n", flavorName, pkgMgr)
		if resp.GetIsFullyCompliant() {
			fmt.Fprintf(rootOpts.Out, "Compliance Status: FULLY COMPLIANT (Installed: %d, Missing: %d)\n",
				resp.GetInstalledCount(), resp.GetMissingCount())
		} else {
			fmt.Fprintf(rootOpts.Out, "Compliance Status: ACTION REQUIRED (Installed: %d, Missing: %d)\n",
				resp.GetInstalledCount(), resp.GetMissingCount())
		}
		fmt.Fprintln(rootOpts.Out, "================================================================================")
		fmt.Fprintln(rootOpts.Out)

		// Page 1: MISSING PREREQUISITES
		fmt.Fprintln(rootOpts.Out, "[PAGE 1: MISSING PREREQUISITES]")
		fmt.Fprintln(rootOpts.Out, "--------------------------------------------------------------------------------")
		missing := resp.GetMissingPrerequisites()
		if len(missing) == 0 {
			fmt.Fprintln(rootOpts.Out, "No missing prerequisites! System is 100% compliant with all NAS features.")
			fmt.Fprintln(rootOpts.Out)
		} else {
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tCATEGORY\tKIND\tTARGET\tREQUIRED BY\tINSTALL INSTRUCTION")
			for _, p := range missing {
				reqBy := strings.Join(p.GetRequiredBy(), ", ")
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					p.GetName(), p.GetCategory(), p.GetKind(), p.GetBinaryOrPath(), reqBy, p.GetInstallHint())
			}
			tw.Flush()

			if script := resp.GetRemediationScript(); script != "" {
				fmt.Fprintln(rootOpts.Out)
				fmt.Fprintf(rootOpts.Out, ">>> EXACT REMEDIATION COMMAND FOR %s:\n", strings.ToUpper(flavorName))
				fmt.Fprintln(rootOpts.Out, script)
			}
			fmt.Fprintln(rootOpts.Out)
		}

		// Page 2: INSTALLED PREREQUISITES
		fmt.Fprintln(rootOpts.Out, "[PAGE 2: INSTALLED PREREQUISITES]")
		fmt.Fprintln(rootOpts.Out, "--------------------------------------------------------------------------------")
		installed := resp.GetInstalledPrerequisites()
		if len(installed) == 0 {
			fmt.Fprintln(rootOpts.Out, "No native prerequisites found.")
		} else {
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tCATEGORY\tKIND\tVERSION\tRESOLVED TARGET\tDETAILS")
			for _, p := range installed {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					p.GetName(), p.GetCategory(), p.GetKind(), p.GetVersion(), p.GetBinaryOrPath(), p.GetDetails())
			}
			tw.Flush()
		}
		fmt.Fprintln(rootOpts.Out)
		return nil
	}

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Trigger a live re-verification of all system prerequisites and display status report",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.CheckPrerequisites(cmd.Context(), &nasv1.CheckPrerequisitesRequest{})
			if err != nil {
				return err
			}
			return printReport(resp)
		},
	}

	reportCmd := &cobra.Command{
		Use:   "report",
		Short: "Display compliance status report with missing list first and exact Linux flavor remediation instructions",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.CheckPrerequisites(cmd.Context(), &nasv1.CheckPrerequisitesRequest{})
			if err != nil {
				return err
			}
			return printReport(resp)
		},
	}

	cmd.AddCommand(listCmd, checkCmd, reportCmd)
	return cmd
}

func newZfsCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "zfs",
		Short: "Manage ZFS pools, datasets, and volumes",
	}

	poolsCmd := &cobra.Command{
		Use:   "pools",
		Short: "List ZFS storage pools",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.ListZfsPools(cmd.Context(), &nasv1.ListZfsPoolsRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetZfsPools())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tSTATE\tLAYOUT\tCAPACITY\tMOUNTPOINT\tCOMPRESSION")
			for _, p := range resp.GetZfsPools() {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d%%\t%s\t%s\n",
					p.GetName(), p.GetState(), p.GetVdevLayout(), p.GetCapacityPercent(), p.GetMountpoint(), p.GetCompression())
			}
			return tw.Flush()
		},
	}


	scrubCmd := &cobra.Command{
		Use:   "scrub <pool-id>",
		Short: "Start a ZFS scrub on a pool (:scrub)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			name := ensurePrefix(args[0], "zfsPools/")
			pool, err := client.ScrubZfsPool(cmd.Context(), &nasv1.ScrubZfsPoolRequest{Name: name})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(pool)
			}
			fmt.Fprintf(rootOpts.Out, "Scrubbed %s: %s\n", pool.GetName(), pool.GetLastScrubStatus())
			return nil
		},
	}

	datasetsCmd := &cobra.Command{
		Use:   "datasets <pool-id>",
		Short: "List ZFS datasets in a pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			parent := ensurePrefix(args[0], "zfsPools/")
			resp, err := client.ListZfsDatasets(cmd.Context(), &nasv1.ListZfsDatasetsRequest{Parent: parent})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetZfsDatasets())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tMOUNTPOINT\tCOMPRESSION\tRECORDSIZE\tSNAPSHOTS")
			for _, ds := range resp.GetZfsDatasets() {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\n",
					ds.GetName(), ds.GetMountpoint(), ds.GetCompression(), ds.GetRecordsize(), ds.GetSnapshotsCount())
			}
			return tw.Flush()
		},
	}


	volumesCmd := &cobra.Command{
		Use:   "volumes <pool-id>",
		Short: "List ZFS volumes (ZVols) in a pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			parent := ensurePrefix(args[0], "zfsPools/")
			resp, err := client.ListZfsVolumes(cmd.Context(), &nasv1.ListZfsVolumesRequest{Parent: parent})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetZfsVolumes())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tDEVICE\tBLOCKSIZE\tCOMPRESSION\tTHIN")
			for _, v := range resp.GetZfsVolumes() {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%v\n",
					v.GetName(), v.GetDevicePath(), v.GetVolblocksize(), v.GetCompression(), v.GetThinProvisioned())
			}
			return tw.Flush()
		},
	}


	var forceExport bool
	exportCmd := &cobra.Command{
		Use:   "export <pool-id>",
		Short: "Export a ZFS storage pool (:export)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			name := ensurePrefix(args[0], "zfsPools/")
			_, err = client.ExportZfsPool(cmd.Context(), &nasv1.ExportZfsPoolRequest{Name: name, Force: forceExport})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "Exported %s successfully\n", name)
			return nil
		},
	}
	exportCmd.Flags().BoolVarP(&forceExport, "force", "f", false, "Force export even if busy")

	cmd.AddCommand(poolsCmd, scrubCmd, exportCmd, datasetsCmd, volumesCmd)
	return cmd
}

func newVMCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vm",
		Short: "Manage KVM virtual machines and noVNC consoles",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List KVM virtual machines",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.ListVirtualMachines(cmd.Context(), &nasv1.ListVirtualMachinesRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetVirtualMachines())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tSTATE\tCPU\tRAM(MiB)\tDISKS\tIP ADDRESSES\tFIRMWARE")
			for _, vm := range resp.GetVirtualMachines() {
				ips := "-"
				if len(vm.GetGuestIpAddresses()) > 0 {
					ips = strings.Join(vm.GetGuestIpAddresses(), ", ")
				}
				disksInfo := fmt.Sprintf("%d disk(s)", len(vm.GetDisks()))
				if len(vm.GetDisks()) == 0 && vm.GetDiskPath() != "" {
					disksInfo = vm.GetDiskPath()
				}
				cpuInfo := fmt.Sprintf("%dvCPU", vm.GetVcpus())
				if vm.GetSockets() > 0 && vm.GetCores() > 0 {
					cpuInfo = fmt.Sprintf("%dvCPU (%ds/%dc)", vm.GetVcpus(), vm.GetSockets(), vm.GetCores())
				}
				firmware := vm.GetBiosType()
				if firmware == "" {
					firmware = "seabios"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
					vm.GetName(), vm.GetState(), cpuInfo, vm.GetMemoryMib(), disksInfo, ips, firmware)
			}
			return tw.Flush()
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <vm-id>",
		Short: "Get detailed Proxmox-grade hardware and telemetry configuration of a VM",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			vm, err := client.GetVirtualMachine(cmd.Context(), &nasv1.GetVirtualMachineRequest{
				Name: ensurePrefix(args[0], "virtualMachines/"),
			})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(vm)
			}
			fmt.Fprintf(rootOpts.Out, "Virtual Machine: %s (%s)\n", vm.GetVmName(), vm.GetUuid())
			fmt.Fprintf(rootOpts.Out, "State:           %s\n", vm.GetState())
			fmt.Fprintf(rootOpts.Out, "CPU:             %d vCPUs (%d sockets, %d cores) [Type: %s]\n",
				vm.GetVcpus(), vm.GetSockets(), vm.GetCores(), vm.GetCpuType())
			fmt.Fprintf(rootOpts.Out, "Memory:          %d MiB (Balloon min: %d MiB, Hugepages: %s)\n",
				vm.GetMemoryMib(), vm.GetBalloonMinMib(), vm.GetHugepages())
			fmt.Fprintf(rootOpts.Out, "Firmware:        BIOS=%s, Machine=%s, TPM=%s, SecureBoot=%t\n",
				vm.GetBiosType(), vm.GetMachineType(), vm.GetTpmVersion(), vm.GetSecureBoot())
			fmt.Fprintf(rootOpts.Out, "Display / VNC:   %s | Port %d (%s)\n",
				vm.GetDisplayType(), vm.GetVncPort(), vm.GetVncDisplay())
			fmt.Fprintf(rootOpts.Out, "Autostart:       %t (Startup delay: %ds)\n", vm.GetAutostart(), vm.GetStartupDelaySeconds())
			fmt.Fprintf(rootOpts.Out, "Boot Order:      %s\n", vm.GetBootOrder())
			if vm.GetAgentRunning() {
				fmt.Fprintf(rootOpts.Out, "Guest Agent:     RUNNING (OS: %s, IPs: %s)\n",
					vm.GetGuestOsName(), strings.Join(vm.GetGuestIpAddresses(), ", "))
			}

			if len(vm.GetDisks()) > 0 {
				fmt.Fprintln(rootOpts.Out, "\nDisks:")
				tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "  ID\tBUS\tPATH\tSIZE\tCACHE\tDISCARD\tIOTHREAD")
				for _, d := range vm.GetDisks() {
					fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%t\t%t\n",
						d.GetId(), d.GetBus(), d.GetDiskPath(), domain.FormatBytes(d.GetSizeBytes()),
						d.GetCacheMode(), d.GetDiscard(), d.GetIothread())
				}
				_ = tw.Flush()
			}

			if len(vm.GetNetworkInterfaces()) > 0 {
				fmt.Fprintln(rootOpts.Out, "\nNetwork Interfaces:")
				tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "  ID\tMODEL\tBRIDGE\tVLAN\tMAC\tRATE LIMIT")
				for _, iface := range vm.GetNetworkInterfaces() {
					fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\t%s\t%d Mbps\n",
						iface.GetId(), iface.GetModel(), iface.GetBridge(), iface.GetVlanTag(),
						iface.GetMacAddress(), iface.GetRateLimitMbps())
				}
				_ = tw.Flush()
			}

			if vm.GetCloudInit() != nil && vm.GetCloudInit().GetUser() != "" {
				fmt.Fprintf(rootOpts.Out, "\nCloud-Init:\n  User: %s | IP: %s\n",
					vm.GetCloudInit().GetUser(), vm.GetCloudInit().GetIpConfig())
			}
			return nil
		},
	}

	var forceReboot bool
	rebootCmd := &cobra.Command{
		Use:   "reboot <vm-id>",
		Short: "Reboot a KVM virtual machine gracefully via ACPI (:reboot, or force reset with --force)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			vm, err := client.RebootVirtualMachine(cmd.Context(), &nasv1.RebootVirtualMachineRequest{
				Name:  ensurePrefix(args[0], "virtualMachines/"),
				Force: forceReboot,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "%s is rebooting (%s)\n", vm.GetName(), vm.GetState())
			return nil
		},
	}
	rebootCmd.Flags().BoolVarP(&forceReboot, "force", "f", false, "Force reset immediately without waiting for graceful ACPI reboot")

	resetCmd := &cobra.Command{
		Use:   "reset <vm-id>",
		Short: "Hard reset a KVM virtual machine (:reset)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			vm, err := client.ResetVirtualMachine(cmd.Context(), &nasv1.ResetVirtualMachineRequest{
				Name: ensurePrefix(args[0], "virtualMachines/"),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "%s reset successfully (%s)\n", vm.GetName(), vm.GetState())
			return nil
		},
	}

	pauseCmd := &cobra.Command{
		Use:   "pause <vm-id>",
		Short: "Pause/suspend a running KVM virtual machine (:pause)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			vm, err := client.PauseVirtualMachine(cmd.Context(), &nasv1.PauseVirtualMachineRequest{
				Name: ensurePrefix(args[0], "virtualMachines/"),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "%s paused (%s)\n", vm.GetName(), vm.GetState())
			return nil
		},
	}

	resumeCmd := &cobra.Command{
		Use:   "resume <vm-id>",
		Short: "Resume a paused KVM virtual machine (:resume)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			vm, err := client.ResumeVirtualMachine(cmd.Context(), &nasv1.ResumeVirtualMachineRequest{
				Name: ensurePrefix(args[0], "virtualMachines/"),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "%s resumed (%s)\n", vm.GetName(), vm.GetState())
			return nil
		},
	}

	var (
		cloneName   string
		clonePool   string
		linkedClone bool
	)
	cloneCmd := &cobra.Command{
		Use:   "clone <source-vm-id> <new-vm-id>",
		Short: "Clone a virtual machine with optional linked/full ZFS backing (:clone)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			vm, err := client.CloneVirtualMachine(cmd.Context(), &nasv1.CloneVirtualMachineRequest{
				Name:                ensurePrefix(args[0], "virtualMachines/"),
				NewVirtualMachineId: args[1],
				NewName:             cloneName,
				TargetPool:          clonePool,
				LinkedClone:         linkedClone,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "Successfully cloned %s to %s (%s)\n", args[0], vm.GetName(), vm.GetState())
			return nil
		},
	}
	cloneCmd.Flags().StringVar(&cloneName, "name", "", "Display name for the new cloned VM")
	cloneCmd.Flags().StringVar(&clonePool, "pool", "tank", "Target ZFS pool for cloned disks")
	cloneCmd.Flags().BoolVar(&linkedClone, "linked", true, "Create a linked clone based on ZFS snapshot")

	// Snapshot subcommands
	snapCmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Manage virtual machine live and offline snapshots",
	}

	snapListCmd := &cobra.Command{
		Use:   "list <vm-id>",
		Short: "List snapshots of a virtual machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.ListVirtualMachineSnapshots(cmd.Context(), &nasv1.ListVirtualMachineSnapshotsRequest{
				Parent: ensurePrefix(args[0], "virtualMachines/"),
			})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetSnapshots())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "SNAPSHOT ID\tVM STATE\tRAM INCLUDED\tCREATED\tDESCRIPTION")
			for _, s := range resp.GetSnapshots() {
				createdStr := s.GetCreateTime().AsTime().Format("2006-01-02 15:04:05")
				fmt.Fprintf(tw, "%s\t%s\t%t\t%s\t%s\n",
					s.GetSnapshotId(), s.GetVmState(), s.GetIncludeRam(), createdStr, s.GetDescription())
			}
			return tw.Flush()
		},
	}

	var (
		snapDesc       string
		snapIncludeRAM bool
	)
	snapCreateCmd := &cobra.Command{
		Use:   "create <vm-id> <snapshot-id>",
		Short: "Create a new snapshot of a virtual machine",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			snap, err := client.CreateVirtualMachineSnapshot(cmd.Context(), &nasv1.CreateVirtualMachineSnapshotRequest{
				Parent:     ensurePrefix(args[0], "virtualMachines/"),
				SnapshotId: args[1],
				Snapshot: &nasv1.VirtualMachineSnapshot{
					Description: snapDesc,
					IncludeRam:  snapIncludeRAM,
				},
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "Created snapshot %s for %s\n", snap.GetSnapshotId(), args[0])
			return nil
		},
	}
	snapCreateCmd.Flags().StringVarP(&snapDesc, "description", "d", "", "Snapshot description")
	snapCreateCmd.Flags().BoolVar(&snapIncludeRAM, "ram", false, "Include running VM RAM memory in snapshot")

	snapRollbackCmd := &cobra.Command{
		Use:   "rollback <vm-id> <snapshot-id>",
		Short: "Rollback a virtual machine to a snapshot state",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			snapName := fmt.Sprintf("%s/snapshots/%s", ensurePrefix(args[0], "virtualMachines/"), args[1])
			vm, err := client.RollbackVirtualMachineSnapshot(cmd.Context(), &nasv1.RollbackVirtualMachineSnapshotRequest{
				Name: snapName,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "Successfully rolled back %s to %s (current state: %s)\n", args[0], args[1], vm.GetState())
			return nil
		},
	}

	snapDeleteCmd := &cobra.Command{
		Use:   "delete <vm-id> <snapshot-id>",
		Short: "Delete a virtual machine snapshot",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			snapName := fmt.Sprintf("%s/snapshots/%s", ensurePrefix(args[0], "virtualMachines/"), args[1])
			if _, err := client.DeleteVirtualMachineSnapshot(cmd.Context(), &nasv1.DeleteVirtualMachineSnapshotRequest{
				Name: snapName,
			}); err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "Deleted snapshot %s from %s\n", args[1], args[0])
			return nil
		},
	}

	snapCmd.AddCommand(snapListCmd, snapCreateCmd, snapRollbackCmd, snapDeleteCmd)


	startCmd := &cobra.Command{
		Use:   "start <vm-id>",
		Short: "Start a KVM virtual machine (:start)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			vm, err := client.StartVirtualMachine(cmd.Context(), &nasv1.StartVirtualMachineRequest{
				Name: ensurePrefix(args[0], "virtualMachines/"),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "%s is now %s\n", vm.GetName(), vm.GetState())
			return nil
		},
	}

	var forceStop bool
	stopCmd := &cobra.Command{
		Use:   "stop <vm-id>",
		Short: "Stop a KVM virtual machine (:stop, or force destroy with --force)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			vm, err := client.StopVirtualMachine(cmd.Context(), &nasv1.StopVirtualMachineRequest{
				Name:  ensurePrefix(args[0], "virtualMachines/"),
				Force: forceStop,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "%s is now %s\n", vm.GetName(), vm.GetState())
			return nil
		},
	}
	stopCmd.Flags().BoolVarP(&forceStop, "force", "f", false, "Force stop/destroy immediately without waiting for graceful ACPI shutdown")

	consoleCmd := &cobra.Command{
		Use:   "console <vm-id>",
		Short: "Open a noVNC WebSocket console session for a VM (:openConsole)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			sess, err := client.OpenVirtualMachineConsole(cmd.Context(), &nasv1.OpenVirtualMachineConsoleRequest{
				Name: ensurePrefix(args[0], "virtualMachines/"),
			})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(sess)
			}
			fmt.Fprintf(rootOpts.Out, "VM: %s | WebSocket Path: %s | VNC Port: %d | Expires: %s\n",
				sess.GetVmName(), sess.GetWebsocketUrl(), sess.GetVncPort(), sess.GetExpiresAt())
			return nil
		},
	}

	cmd.AddCommand(listCmd, getCmd, startCmd, stopCmd, rebootCmd, resetCmd, pauseCmd, resumeCmd, cloneCmd, snapCmd, consoleCmd)
	return cmd
}

func newDockerCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docker",
		Short: "Manage Docker containers",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List Docker containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.ListDockerContainers(cmd.Context(), &nasv1.ListDockerContainersRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetDockerContainers())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tIMAGE\tSTATE\tSTATUS\tPORTS")
			for _, c := range resp.GetDockerContainers() {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					c.GetName(), c.GetImage(), c.GetState(), c.GetStatusText(), strings.Join(c.GetPorts(), ","))
			}
			return tw.Flush()
		},
	}

	getCmd := &cobra.Command{
		Use:     "get <container-id>",
		Aliases: []string{"inspect"},
		Short:   "Get details and live telemetry for a Docker container",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			c, err := client.GetDockerContainer(cmd.Context(), &nasv1.GetDockerContainerRequest{
				Name: ensurePrefix(args[0], "dockerContainers/"),
			})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(c)
			}
			fmt.Fprintf(rootOpts.Out, "Name:           %s\n", c.GetName())
			fmt.Fprintf(rootOpts.Out, "Container ID:   %s\n", c.GetContainerId())
			fmt.Fprintf(rootOpts.Out, "Container Name: %s\n", c.GetContainerName())
			fmt.Fprintf(rootOpts.Out, "Image:          %s\n", c.GetImage())
			fmt.Fprintf(rootOpts.Out, "State:          %s\n", c.GetState())
			fmt.Fprintf(rootOpts.Out, "Status:         %s\n", c.GetStatusText())
			fmt.Fprintf(rootOpts.Out, "CPU Usage:      %.2f%%\n", c.GetCpuPercent())
			fmt.Fprintf(rootOpts.Out, "Memory Usage:   %d MiB\n", c.GetMemoryUsageMib())
			fmt.Fprintf(rootOpts.Out, "Network Mode:   %s\n", c.GetNetworkMode())
			fmt.Fprintf(rootOpts.Out, "Restart Policy: %s\n", c.GetRestartPolicy())
			fmt.Fprintf(rootOpts.Out, "Ports:          %s\n", strings.Join(c.GetPorts(), ", "))
			fmt.Fprintf(rootOpts.Out, "Volumes:        %s\n", strings.Join(c.GetVolumes(), ", "))
			fmt.Fprintf(rootOpts.Out, "Created At:     %s\n", c.GetCreatedAt())
			return nil
		},
	}

	startCmd := &cobra.Command{
		Use:   "start <container-id>",
		Short: "Start a Docker container (:start)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			c, err := client.StartDockerContainer(cmd.Context(), &nasv1.StartDockerContainerRequest{
				Name: ensurePrefix(args[0], "dockerContainers/"),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "%s is now %s (%s)\n", c.GetName(), c.GetState(), c.GetStatusText())
			return nil
		},
	}

	stopCmd := &cobra.Command{
		Use:   "stop <container-id>",
		Short: "Stop a Docker container (:stop)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			c, err := client.StopDockerContainer(cmd.Context(), &nasv1.StopDockerContainerRequest{
				Name: ensurePrefix(args[0], "dockerContainers/"),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "%s is now %s (%s)\n", c.GetName(), c.GetState(), c.GetStatusText())
			return nil
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart <container-id>",
		Short: "Restart a Docker container (:restart)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			c, err := client.RestartDockerContainer(cmd.Context(), &nasv1.RestartDockerContainerRequest{
				Name: ensurePrefix(args[0], "dockerContainers/"),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "%s restarted successfully (%s)\n", c.GetName(), c.GetStatusText())
			return nil
		},
	}

	var forceDelete bool
	deleteCmd := &cobra.Command{
		Use:   "delete <container-id>",
		Short: "Remove a Docker container",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			name := ensurePrefix(args[0], "dockerContainers/")
			if _, err := client.DeleteDockerContainer(cmd.Context(), &nasv1.DeleteDockerContainerRequest{
				Name:  name,
				Force: forceDelete,
			}); err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "Deleted %s\n", name)
			return nil
		},
	}
	deleteCmd.Flags().BoolVarP(&forceDelete, "force", "f", false, "Force removal of a running container")

	var (
		createImage   string
		createPorts   []string
		createVolumes []string
		createEnv     []string
		createRestart string
		createNet     string
		createCompose string
	)
	createCmd := &cobra.Command{
		Use:   "create <container-name>",
		Short: "Create and deploy a Docker container (via Docker Compose persistence)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			var composeYAML string
			if createCompose != "" {
				if data, err := os.ReadFile(createCompose); err == nil {
					composeYAML = string(data)
				} else {
					composeYAML = createCompose
				}
			}

			c, err := client.CreateDockerContainer(cmd.Context(), &nasv1.CreateDockerContainerRequest{
				DockerContainerId: args[0],
				DockerContainer: &nasv1.DockerContainer{
					Image:         createImage,
					Ports:         createPorts,
					Volumes:       createVolumes,
					Environment:   createEnv,
					RestartPolicy: createRestart,
					NetworkMode:   createNet,
					ComposeYaml:   composeYAML,
				},
			})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(c)
			}
			fmt.Fprintf(rootOpts.Out, "Created and started container %s (%s) from image %s\n",
				c.GetName(), c.GetContainerId(), c.GetImage())
			return nil
		},
	}
	createCmd.Flags().StringVar(&createImage, "image", "", "OCI container image")
	createCmd.Flags().StringVar(&createCompose, "compose", "", "Path to compose.yaml or inline compose YAML specification")
	createCmd.Flags().StringSliceVarP(&createPorts, "port", "p", nil, "Port mapping (host:container)")
	createCmd.Flags().StringSliceVarP(&createVolumes, "volume", "v", nil, "Volume mount (host:container)")
	createCmd.Flags().StringSliceVarP(&createEnv, "env", "e", nil, "Environment variable (KEY=VAL)")
	createCmd.Flags().StringVar(&createRestart, "restart", "unless-stopped", "Restart policy (unless-stopped, always, on-failure, no)")
	createCmd.Flags().StringVar(&createNet, "net", "bridge", "Network mode (bridge, host)")

	var (
		updateImage   string
		updatePorts   []string
		updateVolumes []string
		updateEnv     []string
		updateRestart string
		updateNet     string
		updateMemory  int64
		updateCompose string
	)
	updateCmd := &cobra.Command{
		Use:   "update <container-name>",
		Short: "Update / reconfigure an existing Docker container (via Docker Compose)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			var paths []string
			cReq := &nasv1.DockerContainer{
				Name: ensurePrefix(args[0], "dockerContainers/"),
			}
			if cmd.Flags().Changed("image") {
				cReq.Image = updateImage
				paths = append(paths, "image")
			}
			if cmd.Flags().Changed("port") {
				cReq.Ports = updatePorts
				paths = append(paths, "ports")
			}
			if cmd.Flags().Changed("volume") {
				cReq.Volumes = updateVolumes
				paths = append(paths, "volumes")
			}
			if cmd.Flags().Changed("env") {
				cReq.Environment = updateEnv
				paths = append(paths, "environment")
			}
			if cmd.Flags().Changed("restart") {
				cReq.RestartPolicy = updateRestart
				paths = append(paths, "restart_policy")
			}
			if cmd.Flags().Changed("net") {
				cReq.NetworkMode = updateNet
				paths = append(paths, "network_mode")
			}
			if cmd.Flags().Changed("memory") {
				cReq.MemoryLimitMib = updateMemory
				paths = append(paths, "memory_limit_mib")
			}
			if cmd.Flags().Changed("compose") {
				var yml string
				if data, err := os.ReadFile(updateCompose); err == nil {
					yml = string(data)
				} else {
					yml = updateCompose
				}
				cReq.ComposeYaml = yml
				paths = append(paths, "compose_yaml")
			}

			c, err := client.UpdateDockerContainer(cmd.Context(), &nasv1.UpdateDockerContainerRequest{
				DockerContainer: cReq,
				UpdateMask:      &fieldmaskpb.FieldMask{Paths: paths},
			})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(c)
			}
			fmt.Fprintf(rootOpts.Out, "Updated container %s (%s)\n", c.GetName(), c.GetImage())
			return nil
		},
	}
	updateCmd.Flags().StringVar(&updateImage, "image", "", "OCI container image")
	updateCmd.Flags().StringVar(&updateCompose, "compose", "", "Path to compose.yaml or inline compose YAML specification")
	updateCmd.Flags().StringSliceVarP(&updatePorts, "port", "p", nil, "Port mapping (host:container)")
	updateCmd.Flags().StringSliceVarP(&updateVolumes, "volume", "v", nil, "Volume mount (host:container)")
	updateCmd.Flags().StringSliceVarP(&updateEnv, "env", "e", nil, "Environment variable (KEY=VAL)")
	updateCmd.Flags().StringVar(&updateRestart, "restart", "", "Restart policy")
	updateCmd.Flags().StringVar(&updateNet, "net", "", "Network mode")
	updateCmd.Flags().Int64VarP(&updateMemory, "memory", "m", 0, "Memory limit in MiB")

	var tailLines int32
	logsCmd := &cobra.Command{
		Use:   "logs <container-id>",
		Short: "Fetch live logs for a Docker container",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.GetDockerContainerLogs(cmd.Context(), &nasv1.GetDockerContainerLogsRequest{
				Name: ensurePrefix(args[0], "dockerContainers/"),
				Tail: tailLines,
			})
			if err != nil {
				return err
			}
			fmt.Fprint(rootOpts.Out, resp.GetLogs())
			return nil
		},
	}
	logsCmd.Flags().Int32VarP(&tailLines, "tail", "n", 100, "Number of lines to show from the end of the logs")

	cmd.AddCommand(listCmd, getCmd, startCmd, stopCmd, restartCmd, deleteCmd, createCmd, updateCmd, logsCmd)
	return cmd
}

func newLxcCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lxc",
		Short: "Manage LXC system containers",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List LXC containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.ListLxcContainers(cmd.Context(), &nasv1.ListLxcContainersRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetLxcContainers())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tSTATE\tDISTRO\tRELEASE\tIPV4\tMEMORY(MiB)")
			for _, c := range resp.GetLxcContainers() {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\n",
					c.GetName(), c.GetState(), c.GetDistribution(), c.GetRelease(), c.GetIpv4Address(), c.GetMemoryLimitMib())
			}
			return tw.Flush()
		},
	}


	cmd.AddCommand(listCmd)
	return cmd
}

func newShareCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "share",
		Short: "Manage CIFS (Samba) and WebDAV folder shares",
	}

	cifsCmd := &cobra.Command{
		Use:   "cifs",
		Short: "List CIFS/Samba shares",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.ListCifsShares(cmd.Context(), &nasv1.ListCifsSharesRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetCifsShares())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tPATH\tREADONLY\tGUEST\tENABLED")
			for _, s := range resp.GetCifsShares() {
				fmt.Fprintf(tw, "%s\t%s\t%v\t%v\t%v\n",
					s.GetName(), s.GetPath(), s.GetReadOnly(), s.GetGuestOk(), s.GetEnabled())
			}
			return tw.Flush()
		},
	}


	webdavCmd := &cobra.Command{
		Use:   "webdav",
		Short: "List WebDAV shares",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.ListWebdavShares(cmd.Context(), &nasv1.ListWebdavSharesRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetWebdavShares())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tURL PREFIX\tPATH\tREADONLY\tENABLED")
			for _, s := range resp.GetWebdavShares() {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%v\n",
					s.GetName(), s.GetUrlPrefix(), s.GetPath(), s.GetReadOnly(), s.GetEnabled())
			}
			return tw.Flush()
		},
	}


	cmd.AddCommand(cifsCmd, webdavCmd)
	return cmd
}

func newUserCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage Linux system users and SSH access",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List Linux users",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.ListUsers(cmd.Context(), &nasv1.ListUsersRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetUsers())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tUID\tGROUP\tSHELL\tSSH\tSSH KEYS")
			for _, u := range resp.GetUsers() {
				fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%v\t%d\n",
					u.GetName(), u.GetUid(), u.GetPrimaryGroup(), u.GetShell(), u.GetSshEnabled(), len(u.GetSshAuthorizedKeys()))
			}
			return tw.Flush()
		},
	}


	cmd.AddCommand(listCmd)
	return cmd
}

func newGroupCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Manage Linux system groups",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List Linux groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()
			resp, err := client.ListGroups(cmd.Context(), &nasv1.ListGroupsRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetGroups())
			}
			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tGID\tMEMBERS")
			for _, g := range resp.GetGroups() {
				fmt.Fprintf(tw, "%s\t%d\t%s\n",
					g.GetName(), g.GetGid(), strings.Join(g.GetMembers(), ","))
			}
			return tw.Flush()
		},
	}


	cmd.AddCommand(listCmd)
	return cmd
}

func newAppCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "app",
		Aliases: []string{"apps"},
		Short:   "Manage TrueNAS SCALE style Docker Compose Applications",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all deployed Docker Compose Applications",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			resp, err := client.ListDockerApps(cmd.Context(), &nasv1.ListDockerAppsRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetDockerApps())
			}

			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "APP ID\tTITLE\tSTATE\tCONTAINERS\tPORTS\tWEB UI")
			for _, a := range resp.GetDockerApps() {
				containersDesc := fmt.Sprintf("%d containers", len(a.GetContainers()))
				portsStr := strings.Join(a.GetPortMappings(), ", ")
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					a.GetAppId(), a.GetTitle(), a.GetState(), containersDesc, portsStr, a.GetWebUiUrl())
			}
			return tw.Flush()
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <app-id>",
		Short: "Get details, child containers, and compose descriptor of a Docker App",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			name := ensurePrefix(args[0], "dockerApps/")
			a, err := client.GetDockerApp(cmd.Context(), &nasv1.GetDockerAppRequest{Name: name})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(a)
			}

			fmt.Fprintf(rootOpts.Out, "App ID:        %s\n", a.GetAppId())
			fmt.Fprintf(rootOpts.Out, "Title:         %s\n", a.GetTitle())
			fmt.Fprintf(rootOpts.Out, "State:         %s (%s)\n", a.GetState(), a.GetStatusText())
			fmt.Fprintf(rootOpts.Out, "Web UI:        %s\n", a.GetWebUiUrl())
			fmt.Fprintf(rootOpts.Out, "CPU Usage:     %.2f%%\n", a.GetTotalCpuPercent())
			fmt.Fprintf(rootOpts.Out, "Memory Usage:  %d MiB\n", a.GetTotalMemoryMib())
			fmt.Fprintf(rootOpts.Out, "Services:      %s\n", strings.Join(a.GetServices(), ", "))
			fmt.Fprintf(rootOpts.Out, "Compose Path:  %s\n", a.GetComposeFilePath())

			if len(a.GetContainers()) > 0 {
				fmt.Fprintln(rootOpts.Out, "\nContainers:")
				tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "  SERVICE\tCONTAINER NAME\tIMAGE\tSTATE\tCPU\tRAM\tPORTS")
				for _, c := range a.GetContainers() {
					fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%.1f%%\t%d MiB\t%s\n",
						c.GetServiceName(), c.GetName(), c.GetImage(), c.GetState(),
						c.GetCpuPercent(), c.GetMemoryMib(), strings.Join(c.GetPorts(), ","))
				}
				_ = tw.Flush()
			}

			if a.GetComposeYaml() != "" {
				fmt.Fprintln(rootOpts.Out, "\nCompose Descriptor:")
				fmt.Fprintln(rootOpts.Out, a.GetComposeYaml())
			}
			return nil
		},
	}

	var appCompose, appTitle string
	createCmd := &cobra.Command{
		Use:   "create <app-id>",
		Short: "Deploy a Docker Compose Application descriptor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			if appCompose == "" {
				return fmt.Errorf("--compose is required (path to compose.yaml or raw YAML)")
			}
			composeYAML := appCompose
			if data, err := os.ReadFile(appCompose); err == nil {
				composeYAML = string(data)
			}

			a, err := client.CreateDockerApp(cmd.Context(), &nasv1.CreateDockerAppRequest{
				DockerAppId: args[0],
				DockerApp: &nasv1.DockerApp{
					Name:        domain.DockerAppName(args[0]),
					Title:       appTitle,
					ComposeYaml: composeYAML,
				},
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "Created and deployed Docker App %s (%s) with %d containers\n",
				a.GetAppId(), a.GetState(), len(a.GetContainers()))
			return nil
		},
	}
	createCmd.Flags().StringVar(&appCompose, "compose", "", "Path to compose.yaml file or inline Compose YAML (required)")
	createCmd.Flags().StringVar(&appTitle, "title", "", "Display title for the App")

	updateCmd := &cobra.Command{
		Use:   "update <app-id>",
		Short: "Update and redeploy a Docker Compose Application descriptor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			if appCompose == "" {
				return fmt.Errorf("--compose is required (path to compose.yaml or raw YAML)")
			}
			composeYAML := appCompose
			if data, err := os.ReadFile(appCompose); err == nil {
				composeYAML = string(data)
			}

			name := ensurePrefix(args[0], "dockerApps/")
			a, err := client.UpdateDockerApp(cmd.Context(), &nasv1.UpdateDockerAppRequest{
				DockerApp: &nasv1.DockerApp{
					Name:        name,
					Title:       appTitle,
					ComposeYaml: composeYAML,
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"compose_yaml", "title"}},
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "Updated and redeployed Docker App %s (%s)\n", a.GetAppId(), a.GetState())
			return nil
		},
	}
	updateCmd.Flags().StringVar(&appCompose, "compose", "", "Path to compose.yaml file or inline Compose YAML (required)")
	updateCmd.Flags().StringVar(&appTitle, "title", "", "Display title for the App")

	startCmd := &cobra.Command{
		Use:   "start <app-id>",
		Short: "Start all containers in a Docker App",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			name := ensurePrefix(args[0], "dockerApps/")
			a, err := client.StartDockerApp(cmd.Context(), &nasv1.StartDockerAppRequest{Name: name})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "App %s is now %s\n", a.GetAppId(), a.GetState())
			return nil
		},
	}

	stopCmd := &cobra.Command{
		Use:   "stop <app-id>",
		Short: "Stop all containers in a Docker App",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			name := ensurePrefix(args[0], "dockerApps/")
			a, err := client.StopDockerApp(cmd.Context(), &nasv1.StopDockerAppRequest{Name: name})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "App %s is now %s\n", a.GetAppId(), a.GetState())
			return nil
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart <app-id>",
		Short: "Restart all containers in a Docker App",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			name := ensurePrefix(args[0], "dockerApps/")
			a, err := client.RestartDockerApp(cmd.Context(), &nasv1.RestartDockerAppRequest{Name: name})
			if err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "App %s restarted successfully (%s)\n", a.GetAppId(), a.GetState())
			return nil
		},
	}

	var logsTail int
	var logsService string
	logsCmd := &cobra.Command{
		Use:   "logs <app-id>",
		Short: "Fetch aggregated logs for a Docker App",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			name := ensurePrefix(args[0], "dockerApps/")
			resp, err := client.GetDockerAppLogs(cmd.Context(), &nasv1.GetDockerAppLogsRequest{
				Name:        name,
				Tail:        int32(logsTail),
				ServiceName: logsService,
			})
			if err != nil {
				return err
			}
			fmt.Fprint(rootOpts.Out, resp.GetLogs())
			return nil
		},
	}
	logsCmd.Flags().IntVarP(&logsTail, "tail", "n", 100, "Number of log lines to show from the end of the logs")
	logsCmd.Flags().StringVarP(&logsService, "service", "c", "", "Filter logs by service name")

	var deleteVolumes bool
	deleteCmd := &cobra.Command{
		Use:   "delete <app-id>",
		Short: "Delete a Docker Compose App and tear down all its resources",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			name := ensurePrefix(args[0], "dockerApps/")
			if _, err := client.DeleteDockerApp(cmd.Context(), &nasv1.DeleteDockerAppRequest{
				Name:          name,
				DeleteVolumes: deleteVolumes,
			}); err != nil {
				return err
			}
			fmt.Fprintf(rootOpts.Out, "Deleted Docker App %s\n", name)
			return nil
		},
	}
	deleteCmd.Flags().BoolVarP(&deleteVolumes, "volumes", "v", false, "Remove persistent anonymous and named volumes declared in compose")

	datasetCmd := &cobra.Command{
		Use:     "dataset [dataset-name]",
		Aliases: []string{"pool", "storage"},
		Short:   "View or configure the Docker owned ZFS dataset for apps and volumes",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			if len(args) == 1 {
				updated, err := client.UpdateDockerConfig(cmd.Context(), &nasv1.UpdateDockerConfigRequest{
					DockerConfig: &nasv1.DockerConfig{
						Dataset: args[0],
					},
				})
				if err != nil {
					return err
				}
				if rootOpts.JSONOutput {
					return rootOpts.printJSON(updated)
				}
				fmt.Fprintf(rootOpts.Out, "Configured Docker owned ZFS dataset: %s\n", updated.GetDataset())
				fmt.Fprintf(rootOpts.Out, "  Parent Pool:          %s\n", updated.GetPool())
				fmt.Fprintf(rootOpts.Out, "  Mountpoint:           %s\n", updated.GetMountpoint())
				fmt.Fprintf(rootOpts.Out, "  Appdata Volumes Path: %s\n", updated.GetAppdataPath())
				fmt.Fprintf(rootOpts.Out, "  Docker Data Root:     %s\n", updated.GetDockerDataRoot())
				return nil
			}

			cfg, err := client.GetDockerConfig(cmd.Context(), &nasv1.GetDockerConfigRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(cfg)
			}
			statusStr := "Configured"
			if !cfg.GetDatasetSelected() {
				statusStr = "Default (Not explicitly configured)"
			}
			fmt.Fprintf(rootOpts.Out, "Docker Owned ZFS Dataset: %s (%s)\n", cfg.GetDataset(), statusStr)
			fmt.Fprintf(rootOpts.Out, "  Parent Pool:            %s\n", cfg.GetPool())
			fmt.Fprintf(rootOpts.Out, "  Mountpoint:             %s\n", cfg.GetMountpoint())
			fmt.Fprintf(rootOpts.Out, "  Appdata Volumes Path:   %s\n", cfg.GetAppdataPath())
			fmt.Fprintf(rootOpts.Out, "  Docker Data Root:       %s\n", cfg.GetDockerDataRoot())
			fmt.Fprintf(rootOpts.Out, "  Available Datasets:     %s\n", strings.Join(cfg.GetAvailableDatasets(), ", "))
			fmt.Fprintf(rootOpts.Out, "  Available ZFS Pools:    %s\n", strings.Join(cfg.GetAvailablePools(), ", "))
			return nil
		},
	}

	cmd.AddCommand(listCmd, getCmd, createCmd, updateCmd, startCmd, stopCmd, restartCmd, logsCmd, deleteCmd, datasetCmd)
	return cmd
}

func newCatalogCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "catalog",
		Aliases: []string{"cat"},
		Short:   "Browse and sync TrueNAS SCALE Docker Compose application catalog",
	}

	var trainFilter, categoryFilter, queryFilter string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List applications available in the TrueNAS SCALE catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			resp, err := client.ListCatalogApps(cmd.Context(), &nasv1.ListCatalogAppsRequest{
				Train:    trainFilter,
				Category: categoryFilter,
				Query:    queryFilter,
			})
			if err != nil {
				return err
			}

			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp)
			}

			w := tabwriter.NewWriter(rootOpts.Out, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "APP ID\tTITLE\tTRAIN\tCATEGORY\tVERSION\tDEFAULT PORT")
			for _, app := range resp.GetApps() {
				portStr := "—"
				if app.GetDefaultPort() > 0 {
					portStr = fmt.Sprintf("%d", app.GetDefaultPort())
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					app.GetAppId(),
					app.GetTitle(),
					app.GetTrain(),
					app.GetCategory(),
					app.GetAppVersion(),
					portStr,
				)
			}
			w.Flush()
			if resp.GetLastSyncTime() != "" {
				fmt.Fprintf(rootOpts.Out, "\nTotal apps: %d (Last synced: %s)\n", resp.GetTotalSize(), resp.GetLastSyncTime())
			}
			return nil
		},
	}
	listCmd.Flags().StringVar(&trainFilter, "train", "", "Filter by train (stable, community)")
	listCmd.Flags().StringVar(&categoryFilter, "category", "", "Filter by category")
	listCmd.Flags().StringVarP(&queryFilter, "query", "q", "", "Search query for title, description, or tags")

	var getTrain string
	getCmd := &cobra.Command{
		Use:   "get <app-id>",
		Short: "Get details and resolved Docker Compose YAML for a catalog app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			app, err := client.GetCatalogApp(cmd.Context(), &nasv1.GetCatalogAppRequest{
				AppId: args[0],
				Train: getTrain,
			})
			if err != nil {
				return err
			}

			if rootOpts.JSONOutput {
				return rootOpts.printJSON(app)
			}

			fmt.Fprintf(rootOpts.Out, "App ID:       %s\n", app.GetAppId())
			fmt.Fprintf(rootOpts.Out, "Title:        %s\n", app.GetTitle())
			fmt.Fprintf(rootOpts.Out, "Train:        %s\n", app.GetTrain())
			fmt.Fprintf(rootOpts.Out, "Category:     %s\n", app.GetCategory())
			fmt.Fprintf(rootOpts.Out, "Version:      %s\n", app.GetAppVersion())
			fmt.Fprintf(rootOpts.Out, "Description:  %s\n", app.GetDescription())
			if app.GetHome() != "" {
				fmt.Fprintf(rootOpts.Out, "Homepage:     %s\n", app.GetHome())
			}
			fmt.Fprintf(rootOpts.Out, "\n--- Resolved Docker Compose YAML ---\n%s\n", app.GetComposeYaml())
			return nil
		},
	}
	getCmd.Flags().StringVar(&getTrain, "train", "", "Train name (stable, community)")

	var forceSync bool
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize catalog definitions from upstream TrueNAS SCALE repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			summary, err := client.SyncCatalog(cmd.Context(), &nasv1.SyncCatalogRequest{
				Force: forceSync,
			})
			if err != nil {
				return err
			}

			if rootOpts.JSONOutput {
				return rootOpts.printJSON(summary)
			}

			fmt.Fprintf(rootOpts.Out, "Successfully synchronized catalog:\n")
			fmt.Fprintf(rootOpts.Out, "  Message:            %s\n", summary.GetMessage())
			fmt.Fprintf(rootOpts.Out, "  Total applications: %d\n", summary.GetTotalApps())
			if summary.GetLastSyncTime() != "" {
				fmt.Fprintf(rootOpts.Out, "  Sync timestamp:     %s\n", summary.GetLastSyncTime())
			}
			return nil
		},
	}
	syncCmd.Flags().BoolVarP(&forceSync, "force", "f", false, "Force re-download even if cache is fresh")

	cmd.AddCommand(listCmd, getCmd, syncCmd)
	return cmd
}

func newDiskCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disk",
		Short: "Manage and inspect physical storage disks, SMART health, and ZFS pool mapping",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all detected storage disks",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			resp, err := client.ListDisks(cmd.Context(), &nasv1.ListDisksRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp.GetDisks())
			}

			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tMODEL\tTYPE\tSIZE\tTEMP\tSMART\tOWNING POOL\tDEVICE PATH")
			for _, d := range resp.GetDisks() {
				tempStr := "-"
				if d.GetTemperatureCelsius() > 0 {
					tempStr = fmt.Sprintf("%.1f°C", d.GetTemperatureCelsius())
				}
				owning := d.GetOwningZfsPool()
				if owning == "" {
					owning = "-"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					d.GetDiskId(),
					d.GetModel(),
					d.GetType(),
					domain.FormatBytes(d.GetSizeBytes()),
					tempStr,
					d.GetSmartStatus(),
					owning,
					d.GetDevicePath(),
				)
			}
			return tw.Flush()
		},
	}

	getCmd := &cobra.Command{
		Use:   "get <disk_id>",
		Short: "Get detailed information and partition table for a disk",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			name := ensurePrefix(args[0], "disks/")
			d, err := client.GetDisk(cmd.Context(), &nasv1.GetDiskRequest{Name: name})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(d)
			}

			fmt.Fprintf(rootOpts.Out, "Disk:           %s\n", d.GetDiskId())
			fmt.Fprintf(rootOpts.Out, "Device Path:    %s\n", d.GetDevicePath())
			fmt.Fprintf(rootOpts.Out, "Model:          %s\n", d.GetModel())
			fmt.Fprintf(rootOpts.Out, "Serial:         %s\n", d.GetSerial())
			fmt.Fprintf(rootOpts.Out, "Vendor:         %s\n", d.GetVendor())
			fmt.Fprintf(rootOpts.Out, "Type / Transp:  %s (%s)\n", d.GetType(), d.GetTransport())
			fmt.Fprintf(rootOpts.Out, "Capacity:       %s (%d bytes)\n", domain.FormatBytes(d.GetSizeBytes()), d.GetSizeBytes())
			fmt.Fprintf(rootOpts.Out, "Rotational:     %t\n", d.GetRotational())
			fmt.Fprintf(rootOpts.Out, "Block Sizes:    logical=%d, physical=%d\n", d.GetLogicalBlockSize(), d.GetPhysicalBlockSize())
			fmt.Fprintf(rootOpts.Out, "Temperature:    %.1f°C\n", d.GetTemperatureCelsius())
			fmt.Fprintf(rootOpts.Out, "SMART Status:   %s (Health %d%%)\n", d.GetSmartStatus(), d.GetHealthPercentage())
			if d.GetPowerOnHours() > 0 {
				fmt.Fprintf(rootOpts.Out, "Power On Hours: %d hrs\n", d.GetPowerOnHours())
			}
			if d.GetPowerCycleCount() > 0 {
				fmt.Fprintf(rootOpts.Out, "Power Cycles:   %d\n", d.GetPowerCycleCount())
			}
			if d.GetTotalBytesWritten() > 0 {
				fmt.Fprintf(rootOpts.Out, "Data Written:   %s\n", domain.FormatBytes(d.GetTotalBytesWritten()))
			}
			if d.GetTotalBytesRead() > 0 {
				fmt.Fprintf(rootOpts.Out, "Data Read:      %s\n", domain.FormatBytes(d.GetTotalBytesRead()))
			}
			if d.GetUnsafeShutdowns() > 0 {
				fmt.Fprintf(rootOpts.Out, "Unsafe Offs:    %d\n", d.GetUnsafeShutdowns())
			}
			if d.GetMediaErrors() > 0 {
				fmt.Fprintf(rootOpts.Out, "Media Errors:   %d\n", d.GetMediaErrors())
			}
			if d.GetReallocatedSectors() > 0 {
				fmt.Fprintf(rootOpts.Out, "Realloc Sectors:%d\n", d.GetReallocatedSectors())
			}
			if d.GetPendingSectors() > 0 {
				fmt.Fprintf(rootOpts.Out, "Pending Sectors:%d\n", d.GetPendingSectors())
			}
			if d.GetOwningZfsPool() != "" {
				fmt.Fprintf(rootOpts.Out, "Owning ZFS Pool:%s\n", d.GetOwningZfsPool())
			}

			if len(d.GetAliases()) > 0 {
				fmt.Fprintln(rootOpts.Out, "\nAliases & Persistent Paths:")
				for _, a := range d.GetAliases() {
					fmt.Fprintf(rootOpts.Out, "  - %s\n", a)
				}
			}

			if len(d.GetSmartParameters()) > 0 {
				fmt.Fprintln(rootOpts.Out, "\nSMART Parameters:")
				tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "  ID\tNAME\tVALUE\tWORST\tTHRESH\tRAW\tSTATUS\tTYPE")
				for _, sp := range d.GetSmartParameters() {
					fmt.Fprintf(tw, "  #%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
						sp.GetId(), sp.GetName(), sp.GetValue(), sp.GetWorst(), sp.GetThreshold(), sp.GetRawValue(), sp.GetStatus(), sp.GetType())
				}
				_ = tw.Flush()
			}

			if len(d.GetPartitions()) > 0 {
				fmt.Fprintln(rootOpts.Out, "\nPartitions:")
				tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "  PARTITION\tPATH\tSIZE\tMOUNTPOINT\tOWNING POOL")
				for _, p := range d.GetPartitions() {
					fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n",
						p.GetName(), p.GetDevicePath(), domain.FormatBytes(p.GetSizeBytes()), p.GetMountpoint(), p.GetOwningZfsPool())
				}
				_ = tw.Flush()
			}
			return nil
		},
	}

	cmd.AddCommand(listCmd, getCmd)
	return cmd
}

func newSystemCmd(rootOpts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Manage system configuration, main ZFS pool, and dataset lifecycle",
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Display system configuration, main pool, and module states",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			resp, err := client.GetSystemConfig(cmd.Context(), &nasv1.GetSystemConfigRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp)
			}

			fmt.Fprintln(rootOpts.Out, "================================================================================")
			fmt.Fprintln(rootOpts.Out, "SYSTEM CONFIGURATION & POOL STATUS")
			fmt.Fprintln(rootOpts.Out, "================================================================================")
			mainPool := resp.GetMainPool()
			if mainPool == "" {
				fmt.Fprintln(rootOpts.Out, "Main Storage Pool : [UNCONFIGURED] (Only ZFS is enabled)")
			} else {
				fmt.Fprintf(rootOpts.Out, "Main Storage Pool : %s (Initialized: %v)\n", mainPool, resp.GetIsInitialized())
				fmt.Fprintf(rootOpts.Out, "System Dataset    : %s\n", resp.GetSystemDataset())
				fmt.Fprintf(rootOpts.Out, "Mountpoint        : %s\n", resp.GetSystemMountpoint())
			}
			fmt.Fprintf(rootOpts.Out, "Config File Path  : %s\n", resp.GetConfigFilePath())
			fmt.Fprintf(rootOpts.Out, "Enabled Modules   : %s\n", strings.Join(resp.GetEnabledModules(), ", "))
			fmt.Fprintln(rootOpts.Out)
			return nil
		},
	}

	selectPoolCmd := &cobra.Command{
		Use:   "select-pool <pool_name>",
		Short: "Designate the main ZFS storage pool, initialize system datasets, and enable all modules",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			poolName := args[0]
			resp, err := client.SelectMainPool(cmd.Context(), &nasv1.SelectMainPoolRequest{
				PoolName:           poolName,
				AutoImportExisting: true,
			})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp)
			}

			fmt.Fprintf(rootOpts.Out, "Successfully configured %q as Main Storage Pool!\n", resp.GetMainPool())
			fmt.Fprintf(rootOpts.Out, "System Dataset : %s\n", resp.GetSystemDataset())
			fmt.Fprintf(rootOpts.Out, "Mountpoint     : %s\n", resp.GetSystemMountpoint())
			fmt.Fprintf(rootOpts.Out, "Enabled Modules: %s\n", strings.Join(resp.GetEnabledModules(), ", "))
			return nil
		},
	}

	discoverCmd := &cobra.Command{
		Use:   "discover",
		Short: "Scan and discover ZFS pools with existing NAS system configurations",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn, err := rootOpts.dialClient(cmd.Context())
			if err != nil {
				return err
			}
			defer conn.Close()

			resp, err := client.DiscoverPools(cmd.Context(), &nasv1.DiscoverPoolsRequest{})
			if err != nil {
				return err
			}
			if rootOpts.JSONOutput {
				return rootOpts.printJSON(resp)
			}

			tw := tabwriter.NewWriter(rootOpts.Out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "POOL NAME\tIMPORTED\tHAS .NAS-SYSTEM\tDATASET PATH\tMOUNTPOINT")
			for _, p := range resp.GetPools() {
				fmt.Fprintf(tw, "%s\t%v\t%v\t%s\t%s\n",
					p.GetPoolName(), p.GetIsImported(), p.GetHasNasSystem(), p.GetDatasetPath(), p.GetMountpoint())
			}
			return tw.Flush()
		},
	}

	cmd.AddCommand(statusCmd, selectPoolCmd, discoverCmd)
	return cmd
}

func ensurePrefix(val, prefix string) string {
	if strings.HasPrefix(val, prefix) {
		return val
	}
	return prefix + val
}
