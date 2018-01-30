# Tast Chrome OS Infra Integration

[TOC]

## Building

### System Images

The [virtual/target-chromium-os-test] Portage package includes
[chromeos-base/tast-local-test-runner] and [chromeos-base/tast-local-tests-cros]
in its `RDEPEND` list so that the `local_test_runner` executable and the `cros`
local test bundle will be included in `test` system images. Note that these
packages' files are installed under `/usr/local` rather than `/usr` as a result
of being installed for testing.

### Chroot

The [virtual/target-chromium-os-sdk] Portage package includes
[chromeos-base/tast-cmd] and [chromeos-base/tast-remote-tests-cros] in its
`RDEPEND` list so that the `tast` command and the `cros` remote test bundle will
be available in developers' chroots. This also has the effect of making these
files available on VM builders that have their own chroots.

### Lab

The [InfraGoBuilder] cbuildbot builder runs the `EmergeInfraGoBinariesStage`,
`PackageInfraGoBinariesStage`, and `RegisterInfraGoPackagesStage` stages from
[infra_stages.py] to build the [chromeos-base/tast-cmd] and
[chromeos-base/tast-remote-tests-cros] packages and upload them as [CIPD]
packages named `chromiumos/infra/tast-cmd` and
`chromiumos/infra/tast-remote-tests-cros`.

[luci-scheduler.cfg] on [chromite's infra/config branch] defines
`chromeos-infra-go-trigger-tast` and `chromeos-infra-go-trigger-tast-tests`
triggers that are responsible for building the CIPD packages when the [tast
repository] or [tast-tests repository] is updated.

The [staging_lab_task.py] script (internal link) updates the CIPD packages'
`staging` refs to point at the `latest` packages and updates `prod-next` refs to
point at `staging` packages. This process runs automatically.

The [automated_deploy.py] script updates `prod` refs to point at `prod-next`
packages. This process runs when the [Infrastructure Deputy] pushes updated code
to prod.

Finally, the versions of the CIPD packages to use are defined in [cipd.yaml]
(internal link), and the packages are installed under `/opt/infra-tools` on prod
systems via the [infra_tools.pp] Puppet config (internal link). Packages are
deployed automatically after `prod` refs are updated.

### Moblab

[Moblab] images include the [autotest-server] Portage package, which lists
the `tast-cmd` and `tast-remote-tests-cros` packages in its `RDEPEND` section so
their files will be made available to the `tast_Runner` Autotest test (described
below).

## Running

### VMs

The [TastVMTestStage] cbuildbot stage runs Tast tests against virtual machines.
It executes the [cros_run_tast_vm_test] script, which starts a VM and runs the
`tast` command within the chroot.

BVT tests currently run in VMs on the [betty-release] builder (internal link).

### Lab

The [tast_Runner] Autotest server test runs on shards in the lab in order to run
Tast-based tests against lab DUTs. `tast_Runner` executes the `tast` command (as
installed via CIPD) locally and parses its results, raising an exception if any
Tast tests failed. It uses the `~/.ssh/testing_rsa` SSH key described in the
[SSH Test Keys Setup] document.

When [Server-Side Packaging] (internal link) is being used, Autotest server
tests are run within a container. [ssp_deploy_shadow_config.json] (internal
link) contains a stanza to mount the `/opt/infra-tools` directory into the
container, but `tast_Runner`'s control files currently set `REQUIRE_SSP = False`
to disable server-side packaging.

`tast_Runner` is currently configured to run as part of the `bvt-perbuild` suite
on canary builders (i.e. the `-release` builders listed on the [internal
waterfall]). When one or more Tast tests fail, `tast_Runner` reports failure.
The Tast results directory is placed within the Autotest results for
`tast_Runner`, i.e. `tast_Runner/results/` in the DUT's Cloud Storage directory.

### Chroot

`tast_Runner` can also be executed within a chroot using `test_that`, although
developers are better off [running the tast command directly] in that case.

[virtual/target-chromium-os-test]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/virtual/target-chromium-os-test/target-chromium-os-test-1.ebuild
[chromeos-base/tast-local-test-runner]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-local-test-runner/tast-local-test-runner-9999.ebuild
[chromeos-base/tast-local-tests-cros]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-local-tests-cros/tast-local-tests-cros-9999.ebuild
[virtual/target-chromium-os-sdk]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/virtual/target-chromium-os-sdk/target-chromium-os-sdk-1.ebuild
[chromeos-base/tast-cmd]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-cmd/tast-cmd-9999.ebuild
[chromeos-base/tast-remote-tests-cros]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/tast-remote-tests-cros/tast-remote-tests-cros-9999.ebuild
[InfraGoBuilder]: https://chromium.googlesource.com/chromiumos/chromite/+/master/cbuildbot/builders/infra_builders.py
[infra_stages.py]: https://chromium.googlesource.com/chromiumos/chromite/+/master/cbuildbot/stages/infra_stages.py
[CIPD]: https://github.com/luci/luci-go/tree/master/cipd
[luci-scheduler.cfg]: https://chromium.googlesource.com/chromiumos/chromite/+/infra/config/luci-scheduler.cfg
[chromite's infra/config branch]: https://chromium.googlesource.com/chromiumos/chromite/+/infra/config
[tast repository]: https://chromium.googlesource.com/chromiumos/platform/tast
[tast-tests repository]: https://chromium.googlesource.com/chromiumos/platform/tast-tests
[staging_lab_task.py]: https://chrome-internal.googlesource.com/chromeos/chromeos-admin/+/master/venv/server_management_lib/tasks/staging_lab_task.py
[automated_deploy.py]: https://chromium.googlesource.com/chromiumos/third_party/autotest/+/master/site_utils/automated_deploy.py
[Infrastructure Deputy]: https://sites.google.com/a/google.com/chromeos/for-team-members/infrastructure/chrome-os-infrastructure-deputy
[cipd.yaml]: https://chrome-internal.googlesource.com/chromeos/chromeos-admin/+/master/puppet/data/cipd.yaml
[infra_tools.pp]: https://chrome-internal.googlesource.com/chromeos/chromeos-admin/+/master/puppet/modules/profiles/manifests/base/infra_tools.pp
[TastVMTestStage]: https://chromium.googlesource.com/chromiumos/chromite/+/master/cbuildbot/stages/tast_test_stages.py
[cros_run_tast_vm_test]: https://chromium.googlesource.com/chromiumos/platform/crostestutils/+/master/cros_run_tast_vm_test
[betty-release]: https://uberchromegw.corp.google.com/i/chromeos/builders/betty-release
[tast_Runner]: https://chromium.googlesource.com/chromiumos/third_party/autotest/+/master/server/site_tests/tast_Runner/tast_Runner.py
[SSH Test Keys Setup]: https://www.chromium.org/chromium-os/testing/autotest-developer-faq/ssh-test-keys-setup
[Server-Side Packaging]: https://sites.google.com/a/google.com/chromeos/for-team-members/infrastructure/server-side-packaging-in-chromeos-test-lab
[ssp_deploy_shadow_config.json]: https://chrome-internal.googlesource.com/chromeos/chromeos-admin/+/master/puppet/modules/lab/files/autotest_shadow_config/ssp_deploy_shadow_config.json
[Moblab]: https://sites.google.com/a/chromium.org/dev/chromium-os/testing/moblab
[autotest-server]: https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/master/chromeos-base/autotest-server/autotest-server-9999.ebuild
[internal waterfall]: https://uberchromegw.corp.google.com/i/chromeos/waterfall
[running the tast command directly]: running_tests.md
