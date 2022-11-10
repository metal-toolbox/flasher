## Flasher - Server fleet firmware install automation.

Flasher is a vendor agnostic tool to automate firmware installats on a server fleet.

Currently supported is Out of band firmware installs, that is - through the server BMC.


## build

`make build-linux`

## run

see [cheatsheet.md](./docs/cheatsheet.md)

## Supported devices

For out of band updates, Flasher uses [bmclib.v2](https://github.com/bmc-toolbox/bmclib/tree/v2) and will out of the box support firmware installs on all devices that bmclib.v2 supports.
