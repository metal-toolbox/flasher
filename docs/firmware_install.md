# flasher update process

In the first release - `v0.1.0`, servers are 'flagged' for updates by an operator,

Flasher periodically queries Serverservice for servers flagged for firmware updates,
and when it has determined the server passes all the update preconditions, the
update install process is initiated.

### Inventory sources

Flasher depends on a device inventory source for the device component data and firmware versions,
the two supported are a `YAML` source and `Serverservice`.


### Firmware install  methods

Flasher determines the firmware to be installed for each component based on one of these three methods, these modes are set at task initialization.

- `PredefinedFirmwareInstallVersions` - firmware versions were predefined.
- `PredefinedFirmwareSet` - firmware set was defined, firmware versions has to be resolved from the set.
- `ResolveFirmwareInstallVersions` (default) - Resolve firmware install versions, since neither a set nor firmware verisons were defined

### PredefinedFirmwareInstallVersions 

In this mode the firmware versions to be installed are included in the install request.

#### firmware configuration

The firmware versions for install in this mode is done by querying requested firmware version from all 
the `Firmware versions` data made available.

Note: The firmware sets are ignored in this mode.

## link serverservice section for install modes
[](serverservice.md)

### ResolveFirmwareInstallVersions

In the `ResolveInstallFirmwareVersions` mode flasher looks at all available 



# Update queue and metadata

# Update pre-requisites

A server is considered eligible for updates when it clears these requirements,

1. A server matches one of the precondition states in the configuration.
2. A server firmware install attribute indicates - the install process is in a `queued` state.
3. A server firmware install attribute indicates - the install process is
   `active`, but has not been updated (last update timestamp) for > a timeout value.


# Trigger an update

An update is triggerred by using a CLI command

`mctl update [-c bmc:<version>|bios:<version>|nic:<model>:<version>] server --id <>`

this CLI program sets up the payload to have the firmware install attribute
set.

An error is returned if,
 - A update is currently in progress.
 - A update is already queued.



# firmware install states

A firmware install status is tracked in the `status` field, 

 - `init`   - Run device precondition checks, resolve firmware versions to be installed
 - `queued` - Queued for execution
 - `active` - Run firmware install actions in task
 - `failed` -
 - `success` -

## ref
https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
