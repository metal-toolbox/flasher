### Serverservice interaction

Serverservice is an inventory source for Flasher.

When Serverservice is enabled through configuration, Flasher queries Serverservice for,

 - Device inventory (includes installed firmware versions).
 - Firmware versions data (includes URLs to download the firmware updates and checksums).
 - Firmware sets - groups of Firmware versions applicable to a device vendor, model type.

#### Task server attribute

In the first release - `v0.1.0`, servers are 'flagged' for updates by an operator,

Flasher periodically queries Serverservice for servers flagged for firmware updates,
and when it has determined the server passes all the update preconditions, the
update install process is initiated.

Servers are flagged by setting a server attribute on a server in the namespace, 
`sh.hollow.flasher.task`. 

The payload for this is in the form,

```json
{
 "namespace": "sh.hollow.flasher.task",
 // this is one of 'out-of-band', 'in-band'
 "method": "out-of-band",
 "status": "",
 // 0 to 3, 3 being the highest
 "priority": 1,
 // user requesting this task
 "requester": "foo",
 ...
}
```

Once flasher identifies and picks up a task, its status attribute is set, initially to `queued`,

```json
{
 "namespace": "sh.hollow.flasher.task",
 "status": "queued",
 "requester": "requester",
 "worker": "some-worker-name"
 ...
}
```

The possible states are `queued`, `active`, `sucess`, `failed`


#### Firmware sets

Firmware sets are firmware versions vetted to be working and applicable for a device/component
based on vendor, model attributes, and going ahead other non device specific attributes, like the organization or project the device is part of.

### Install mode - `PredefinedFirmwareFirmware`

In server services this would be the firmware versions table.

Note: firmware sets are ignored in this mode.

A sample server service flasher install task payload looks like the following,

```json
{
    "namespace": "sh.hollow.flasher.task",
    // user defined install versions
    "Firmware": [
      "bmc": {
               "version": "1.1",
               // these are optional
               "preActions": ["resetBMC"],
               "postActions": ["resetHost"],
               //
               // currenly installed version is not checked - forces downgrades or reinstall of the same firmware
               "force": true
             }
      "bios": {
               "version": "2.6.6".
               ...
      }
    ],
```

### Install mode - `PredefinedFirmwareSet`

In this mode a firmware set name was specified, in the form

```json
{
    "namespace": "sh.hollow.flasher.task",
    // user defined firmware set ID
    "firmwareSet": "dell-r6515",
    ],
    ...
```

Flasher proceeds to lookup the applicable firmware - comparing the versions in the firmware set
and the version installed.

Once its determined the applicable firmware, flasher populates the `Firmware` field. 

```json

{
    "namespace": "sh.hollow.flasher.task",
    "firmwareSet": "dell-r6515",
    // resolved by flasher for this mode.
    "Firmware": [
      "bmc": {
               "version": "1.1",
             }
      "bios": {
               "version": "2.6.6".
               ...
      }
    ],
```


### Install mode - `ResolveFirmwareFirmware`

Flasher looks up firmware sets applicable for the device, components based
on the firmware set labels - `vendor=foo, model=bar`.

It then compares the installed versions on the components with 
the ones available in the set and determines if it should proceed or not.

In this case the task attribute would initially be,

```json
{
 "namespace": "sh.hollow.flasher.task",
 "status": "queued",
 ...
}
```

once the task has been picked, the `firmwareSet`, `Firmware` attributes are populated


```json

{
    "namespace": "sh.hollow.flasher.task",
    // resolved by flasher for this mode.
    "firmwareSet": "dell-r6515",
    // resolved by flasher for this mode.
    "Firmware": [
      "bmc": {
               "version": "1.1",
             }
      "bios": {
               "version": "2.6.6".
               ...
      }
    ],
```


### Flasher states and metadata

Flasher is a stateful application, it maintains an internal queue of firmware install tasks,
at some point if its decided that there is going to be multiple flashers
per facility, then this queueing system needs to be external (redis).

Flasher keeps the state of updates `queued` and `active` by recording the metadata
in Server service as a server attribute in the namespace `sh.hollow.flasher.task`.

Going ahead, this server attribute is referred as the server firmware install attribute.

An example of such firmware install metadata as a server attribute is shown below.

What is not indicated here is that each server attribute contains a created and
last updated timestamp. These timestamps will be used to determine the
'freshness' of an update process.

```json
{
    "namespace": "sh.hollow.flasher.task",
    "firmwareSet": "",
    "Firmware": [
      "bmc": {
               "version": "1.1",
               "updateFileURL": "...",
               "updateFileChecksum": "...",
               "force": true
             }
    ],
    "priority": 0,
    "method": "out-of-band",
    "status": "active",
    "requester": "foobar",
    "worker": "flasher-pod-name",
}
```