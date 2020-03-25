# Hardware Topology

This document describes when each type of hardware feature needs a new topology
value.

[TOC]

## Screen

Changes that require new topology

*   Size change: e.g. 15” vs 12”
*   Native screen resolution changes
*   Technology change: e.g TFT vs IPS vs OLED
*   Touch change: e.g. no touch vs touch

Changes that do not require new topology

*   Vendor supplier change with all same above functionality

## Form Factor

Changes that require new topology

*   Clamshell/Convertible/Detachable/Tablet/Chromebox

Changes that do not require new topology

*   Changing color of plastics and other cosmetic modifications
*   Changing hinge location (assuming convertible type remains the same)

## Audio

Changes that require new topology

*   Same amplifier chip, but different number of speakers
*   Same amplifier chip, same number of speakers, but different placement of
    speakers.
*   Different number of microphones or different placement of microphones
    (A/B/C/D panels)

Changes that do not require new topology

*   Different placement of same audio codec on MLB

## Stylus

Changes that require new topology

*   Presence of stylus support (even if device doesn’t ship with stylus)
*   Garage type of stylus: e.g. garaged internally vs stored externally
*   Wake-on-eject hardware support: e.g. supported vs not supported
*   Technology type: e.g. USI vs EMR

Changes that do not require new topology

*   Where external stylus position is on device

## Keyboard

Changes that require new topology

*   Presence of internal keyboard
*   Any key presence change: e.g. power button present, lock button present,
    number pad presence or not.
*   Presence of backlight

Changes that do not require new topology

*   Moving keys around (e.g. moving Delete key to a different location).
    *   Assuming no EC driver support is needed for this change. If EC driver
        support is needed, then that would require a new topology value
*   Changing Z height of keys or travel for each key press

## Thermal

Changes that may require a new topology

*   More powerful SoC
*   Presence of fan
*   Any changes in a device that may require different thermal tuning and
    consideration

## Camera

Changes that require new topology

*   Different A/B/C/D panel placement of cameras. e.g. 1A-1B vs 1B-1D
    *   Also implies difference in total camera count for system
*   If camera supports ARcore in hardware
*   Different resolution for camera

Changes that do not require new topology

*   Different vendor for camera
*   Different OS driver required for camera

## Accelerometer/Gyroscope/Magnetometer/ProximitySensor {#sensor}

Changes that require new topology

*   Different lid/base placement. e.g. 2lid-1base vs 1lid-2base
*   Number of sensors present on system
*   HW that requires a different EC driver

Changes that do not require new topology

*   Moving sensor sub-board placement within the lid (if it moves out of the
    lid, then we a new topology is required)

## Fingerprint Sensor

Changes that require new topology

*   Number of fingerprint sensors
*   HW interface change

Changes that do not require new topology

*   Placement of sensor on the device (e.g. left side or right side)

## Daughter Board

Changes that require new topology

*   Using a different daughter board

Changes that do not require new topology

*   The length of the cable connecting the MLB and the DB

## Non-Volatile Storage

Changes that require new topology

*   Technology change: e.g. eMMC vs NVMe
*   Component change that requires different FW tuning parameters

Changes that do not require new topology

*   Size of storage: e.g. 32GB vs 128GB

## RAM

Changes that require new topology

*   Change in speed: e.g. 2400MHz vs 3300MHz
*   Change in size: e.g. 8GB vs 16Gb
*   Change in channel count: e.g. Dual vs Single

_NOTE:_ The above list is also used to know when to create a new resistor
strapping RAM\_ID on the SoC. The RAM topology value should mirror the RAM\_ID
resistor strapping. We still want to keep the resistor strapping on the SoC
because we do not want to rely on SoC/EC communication to be working before
enabling RAM access (which would be the case if AP firmware relied on CBI EEPROM
contents proxied through EC)

Changes that do not require new topology

*   Vendor change of part

## WIFI

Changes that require new topology

* Change in bus: e.g. CNVi vs PCIe

Changes that do not require new topology

* Different vendor/part on same bus

## LTE Board

Changes that require new topology

* Presence of LTE board

Changes that do not require new topology

* Second sourced component changes that do not affect FW

## SD Board

Changes that require new topology

* Presence of SD reader board

Changes that do not require new topology

* Second sourced component changes that do not affect FW

## Motherboard USB

Changes that require new topology

* Swapping out USB ICs like TCPCs, PPCs, SSMUXs, or retimers

Changes that do not require new topology

* Adding/Removing isolation diodes on USB lines
