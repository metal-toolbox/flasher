## Tasks
A **Task** represents the flasher work to install one or more firmware on a server.

Tasks may transition through four possible states,
 - **pending**
 - **queued**
 - **active**
 - **failed**
 - **succeeded**

## Actions
Flasher plans and executes an **Action** for each firmware to be installed within a **Task**.

## Steps
A **Step** is the smallest unit of work carried out by flasher as part of an **Action**.

## Flow diagram

The diagram below depicts a flow diagram for a flasher **Task** to install one firmware.
```mermaid
graph TD;
	n5("Initialize");
	n7("Plan");
	n6("Query");
	n8("Run");
	n2("active");
	n10("checkInstalledFirmware");
	n11("downloadFirmware");
	n4("failed");
	n1("pending");
	n13("pollInstallStatus");
	n9("powerOnServer");
	n3("succeeded");
	n12("uploadFirmwareInitiateInstall");
	n5-->n6;
	n7-->|"Plan actions"|n8;
	n6-->|"Installed firmware equals expected"|n3;
	n6-->|"Query for installed firmware"|n7;
	n8-->|"Power on server - if its currently powered off."|n9;
	n2-->|"Invalid task parameters"|n4;
	n2-->n5;
	n10-->|"Task Failed"|n4;
	n10-->|"Download and verify firmware file checksum."|n11;
	n11-->|"Task Failed"|n4;
	n11-->|"Initiate firmware install for component."|n12;
	n1-->|"Task active"|n2;
	n13-->|"Task Failed"|n4;
	n13-->|"Task Successful"|n3;
	n9-->|"Task Failed"|n4;
	n9-->|"Check firmware currently installed on component"|n10;
	n12-->|"Task Failed"|n4;
	n12-->|"Poll BMC for firmware install status until its identified to be in a finalized state."|n13;

```
