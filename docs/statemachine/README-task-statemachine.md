# Flasher task state machine
 
The Task statemachine plans and executes Actions (sub-statemachines) to install firmware.
 
Note: The Task statemachine plans and and executes [Action sub-state machine(s)](docs/statemachine/README-action-statemachine.md) for _each_ firmware being installed.
 
```mermaid
graph TD;
	n2("active");
	n4("failed");
	n1("pending");
	n3("succeeded");
	n2-->|"Task successful"|n3;
	n2-->|"Task failed"|n4;
	n1-->|"Task active"|n2;

```
