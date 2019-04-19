// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"testing"
)

const (
	anotherBootID = "ffffffff-ffff-ffff-ffff-ffffffffffff"
)

func TestReadBootID(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	hst, err := connectToTarget(context.Background(), &td.cfg)
	if err != nil {
		t.Fatal("connectToTarget failed: ", err)
	}

	b, err := readBootID(context.Background(), hst)
	if err != nil {
		t.Fatal("readBootID failed: ", err)
	}
	if b != defaultBootID {
		t.Errorf("readBootID returned %q; want %q", b, defaultBootID)
	}
}

func TestDiagnoseSSHDropNotRecovered(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	if _, err := connectToTarget(context.Background(), &td.cfg); err != nil {
		t.Fatal("connectToTarget failed: ", err)
	}

	// Pass a canceled context to make reconnection fail.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	msg := diagnoseSSHDrop(ctx, &td.cfg)
	const exp = "target did not come back: context canceled"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropNoReboot(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	if _, err := connectToTarget(context.Background(), &td.cfg); err != nil {
		t.Fatal("connectToTarget failed: ", err)
	}

	// boot_id is not changed.

	msg := diagnoseSSHDrop(context.Background(), &td.cfg)
	const exp = "target did not reboot, probably network issue"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropUnknownCrash(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	if _, err := connectToTarget(context.Background(), &td.cfg); err != nil {
		t.Fatal("connectToTarget failed: ", err)
	}

	// Change boot_id to simulate reboot.
	td.bootID = anotherBootID

	msg := diagnoseSSHDrop(context.Background(), &td.cfg)
	const exp = "target rebooted for unknown crash"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropNormalReboot(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	if _, err := connectToTarget(context.Background(), &td.cfg); err != nil {
		t.Fatal("connectToTarget failed: ", err)
	}

	// Simulate normal reboot.
	td.bootID = anotherBootID
	td.journal = `...
Apr 19 07:12:49 pre-shutdown[31389]: Shutting down for reboot: system-update
...
`

	msg := diagnoseSSHDrop(context.Background(), &td.cfg)
	const exp = "target normally shut down for reboot (system-update)"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropKernelCrashBugX86(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	if _, err := connectToTarget(context.Background(), &td.cfg); err != nil {
		t.Fatal("connectToTarget failed: ", err)
	}

	// Simulate sysrq crash on x86.
	td.bootID = anotherBootID
	td.ramOops = `...
[ 2621.828732] Oops: 0002 [#1] PREEMPT SMP
[ 2621.831632] gsmi: Log Shutdown Reason 0x03
[ 2621.831638] Modules linked in: cmac rfcomm uinput snd_soc_kbl_da7219_max98373 snd_soc_da7219 snd_soc_hdac_hdmi snd_soc_skl_ssp_clk snd_soc_dmic snd_soc_skl snd_soc_skl_ipc snd_soc_sst_ipc snd_soc_sst_dsp snd_soc_sst_match snd_hda_ext_core snd_hda_core ipu3_cio2 ipu3_imgu iova videobuf2_dma_sg videobuf2_memops videobuf2_v4l2 videobuf2_core imx355 ak7375 snd_soc_max98373 sx9310 at24 imx319 v4l2_fwnode acpi_als bridge zram stp llc snd_seq_dummy snd_seq snd_seq_device ipt_MASQUERADE nf_nat_masquerade_ipv4 xt_mark fuse iio_trig_sysfs cros_ec_light_prox cros_ec_sensors_sync cros_ec_sensors_ring cros_ec_sensors cros_ec_sensors_core industrialio_triggered_buffer kfifo_buf industrialio ip6table_filter iwlmvm iwlwifi iwl7000_mac80211 cfg80211 hid_google_whiskers hid_google_hammer btusb btrtl cdc_ether btbcm usbnet btintel bluetooth r8152 mii joydev
[ 2621.831749] CPU: 0 PID: 7046 Comm: cros Not tainted 4.4.171-15647-g80b921861fdf #1
[ 2621.831752] Hardware name: Google Nocturne/Nocturne, BIOS Google_Nocturne.10986.3.0 09/02/2018
[ 2621.831756] task: ffff8802556863c0 task.stack: ffff88025eb58000
[ 2621.831759] RIP: 0010:[<ffffffff9a5dd5cf>]  [<ffffffff9a5dd5cf>] sysrq_handle_crash+0x1c/0x26
[ 2621.831765] RSP: 0018:ffff88025eb5bdd8  EFLAGS: 00010246
[ 2621.831768] RAX: 0000000000000000 RBX: ffffffff9ae9db40 RCX: 7b9d00cab8c95b5e
[ 2621.831771] RDX: ffff88027ec11a60 RSI: ffff88027ec0f1c0 RDI: 0000000000000063
[ 2621.831774] RBP: ffff88025eb5bdd8 R08: 0000000000000010 R09: 0000000000000000
[ 2621.831777] R10: 00000000066c0a10 R11: ffffffff9a5dd5b3 R12: 0000000000000007
[ 2621.831780] R13: ffff880065c4b500 R14: 0000000000000063 R15: 0000000000000000
[ 2621.831783] FS:  00007d276393e700(0000) GS:ffff88027ec00000(0000) knlGS:0000000000000000
[ 2621.831786] CS:  0010 DS: 0000 ES: 0000 CR0: 0000000080050033
[ 2621.831789] CR2: 0000000000000000 CR3: 000000025e8a0000 CR4: 0000000000360670
[ 2621.831792] Stack:
[ 2621.831794]  ffff88025eb5be08 ffffffff9a5dd2d8 0000000000000001 000000c00004aef0
[ 2621.831803]  000000c00004aef0 0000000000000001 ffff88025eb5be28 ffffffff9a5de014
[ 2621.831811]  ffff880274b4dcc0 ffff88025eb5bf10 ffff88025eb5be60 ffffffff9a417cf0
[ 2621.831818] Call Trace:
[ 2621.831826]  [<ffffffff9a5dd2d8>] __handle_sysrq+0xa9/0x12b
[ 2621.831830]  [<ffffffff9a5de014>] write_sysrq_trigger+0x3e/0x68
[ 2621.831835]  [<ffffffff9a417cf0>] proc_reg_write+0x50/0x75
[ 2621.831840]  [<ffffffff9a484b5d>] __vfs_write+0xe6/0xf6
[ 2621.831846]  [<ffffffff9a360a9b>] ? __might_sleep+0x41/0x7d
[ 2621.831851]  [<ffffffff9a485001>] SyS_write+0x13f/0x2ae
[ 2621.831857]  [<ffffffff9a9e5ba3>] entry_SYSCALL_64_fastpath+0x31/0xab
[ 2621.831859] Code: 44 00 00 55 48 89 e5 fb e8 70 55 cf ff 5d c3 0f 1f 44 00 00 55 48 89 e5 e8 84 29 d9 ff c7 04 25 10 29 e3 9a 01 00 00 00 0f ae f8 <c6> 04 25 00 00 00 00 01 5d c3 0f 1f 44 00 00 55 48 89 e5 6a 0f
[ 2621.831956] RIP  [<ffffffff9a5dd5cf>] sysrq_handle_crash+0x1c/0x26
[ 2621.831962]  RSP <ffff88025eb5bdd8>
[ 2621.831965] CR2: 0000000000000000
[ 2621.831969] ---[ end trace 9a63ee283dafcc0e ]---
[ 2621.837768] Kernel panic - not syncing: Fatal exception
...
`

	msg := diagnoseSSHDrop(context.Background(), &td.cfg)
	const exp = "kernel crashed in sysrq_handle_crash+0x1c/0x26"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}

func TestDiagnoseSSHDropKernelCrashBugARM(t *testing.T) {
	td := newLocalTestData(t)
	defer td.close()

	if _, err := connectToTarget(context.Background(), &td.cfg); err != nil {
		t.Fatal("connectToTarget failed: ", err)
	}

	// Simulate real crash on ARM.
	td.bootID = anotherBootID
	td.ramOops = `...
[  325.194586] BUG: spinlock cpu recursion on CPU#-1, /0
[  325.194606]  lock: 0xc1056000, .magic: dead4ead, .owner: <none>/-1, .owner_cpu: -1
[  325.194616] CPU: -1 PID: 0 Comm:  Not tainted 3.14.0 #1
[  325.194636] [<c020e51c>] (unwind_backtrace) from [<c020a90c>] (show_stack+0x20/0x24)
[  325.194653] [<c020a90c>] (show_stack) from [<c081e4c8>] (dump_stack+0x7c/0xc0)
[  325.194666] [<c081e4c8>] (dump_stack) from [<c026cdd4>] (spin_dump+0x88/0x9c)
[  325.194678] [<c026cdd4>] (spin_dump) from [<c026ce1c>] (spin_bug+0x34/0x38)
[  325.194688] [<c026ce1c>] (spin_bug) from [<c026cf00>] (do_raw_spin_lock+0x78/0x18c)
[  325.194699] [<c026cf00>] (do_raw_spin_lock) from [<c0823348>] (_raw_spin_lock+0x30/0x34)
[  325.194711] [<c0823348>] (_raw_spin_lock) from [<c0272a34>] (vprintk_emit+0x12c/0x4d4)
[  325.194723] [<c0272a34>] (vprintk_emit) from [<c081d4e4>] (printk+0x4c/0x6c)
[  325.194735] [<c081d4e4>] (printk) from [<c081d194>] (__do_kernel_fault.part.10+0x48/0x84)
[  325.194748] [<c081d194>] (__do_kernel_fault.part.10) from [<c02154dc>] (do_page_fault+0x370/0x3a8)
[  325.194759] [<c02154dc>] (do_page_fault) from [<c02001d0>] (do_DataAbort+0x48/0xc4)
[  325.194769] [<c02001d0>] (do_DataAbort) from [<c020b498>] (__dabt_svc+0x38/0x60)
[  325.194777] Exception stack(0xfff11fa0 to 0xfff11fe8)
[  325.194786] 1fa0: c105828c 00000206 00000193 00000001 fff20190 000002c8 00000206 000002c8
[  325.194797] 1fc0: c02155d0 00000206 c0b35b01 fff200ac fff20010 fff11fec c02154c0 c020b46c
[  325.194806] 1fe0: 60000193 ffffffff
[  325.194814] [<c020b498>] (__dabt_svc) from [<c020b46c>] (__dabt_svc+0xc/0x60)
[  325.194823] Unable to handle kernel paging request at virtual address fff12000
[  325.194831] pgd = c0003000
[  325.194836] [fff12000] *pgd=80000000007003, *pmd=2f7fd003, *pte=00000000
[  325.194852] Internal error: Oops: 207 [#1] PREEMPT SMP ARM
[  325.194859] Modules linked in: ip6t_REJECT veth rfcomm cmac hci_uart i2c_dev btusb btbcm btintel btmrvl_sdio btmrvl uinput bluetooth smsc95xx cdc_ether usbnet r8152 uvcvideo v
ideobuf2_vmalloc bridge stp llc brcmfmac brcmutil zram ipt_MASQUERADE fuse xt_mark snd_seq_dummy cfg80211 ip6table_filter iio_trig_sysfs ip6_tables snd_seq_midi cros_ec_accel snd
_seq_midi_event snd_rawmidi kfifo_buf snd_seq snd_seq_device joydev
[  325.194969] CPU: -1 PID: 0 Comm:  Not tainted 3.14.0 #1
[  325.194978] task: 0edd71f8 ti: fff10000 task.ti:   (null)
[  325.194985] PC is at unwind_frame+0x2ec/0x4b4
[  325.194993] LR is at __start_unwind_tab+0xef0/0x54640
[  325.195001] pc : [<c020e354>]    lr : [<c0c44210>]    psr: 20000193
[  325.195001] sp : fff11c40  ip : 00000001  fp : fff11cc4
[  325.195012] r10: fff12000  r9 : fff11fec  r8 : 00000000
[  325.195019] r7 : c020b46c  r6 : fff12000  r5 : fff11ccc  r4 : fff11c48
[  325.195026] r3 : 000007ff  r2 : 00000005  r1 : 00000200  r0 : 000002c8
[  325.195035] Flags: nzCv  IRQs off  FIQs on  Mode SVC_32  ISA ARM  Segment user
[  325.195043] Control: 30c5387d  Table: 184d70c0  DAC: 2b319fc3
...
`

	msg := diagnoseSSHDrop(context.Background(), &td.cfg)
	// TODO(nya): Improve the symbol extraction. In this case, do_raw_spin_lock or
	// spin_bug seems to be a better choice for diagnosis.
	const exp = "kernel crashed in unwind_frame+0x2ec/0x4b4"
	if msg != exp {
		t.Errorf("diagnoseSSHDrop returned %q; want %q", msg, exp)
	}
}
