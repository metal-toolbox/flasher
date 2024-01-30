#!/bin/sh

set -e

taskSMDoc="docs/statemachine/README-task-statemachine.md"
actionSMDoc="docs/statemachine/README-action-statemachine.md"
actionSMJSON="docs/statemachine/action-statemachine.json"

## generate Task statemachine mermaid graph and docs

echo "generate task statemachine docs..."


echo "# Flasher task state machine" > $taskSMDoc
echo " " >> $taskSMDoc
echo "The Task statemachine plans and executes Actions (sub-statemachines) to install firmware." >> $taskSMDoc
echo " " >> $taskSMDoc
echo "Note: The Task statemachine plans and and executes [Action sub-state machine(s)]($actionSMDoc) for _each_ firmware being installed." >> $taskSMDoc
echo " " >> $taskSMDoc

echo '```mermaid' >> $taskSMDoc
./flasher export-statemachine --task >> $taskSMDoc
echo '```' >> $taskSMDoc


echo "generate task action sub-statemachine docs..."


## generate action statemachine mermaid graph and docs


echo "# Flasher task action sub-state machine(s)" > $actionSMDoc
echo " " >> $actionSMDoc
echo "The task Actions (sub-statemachines) are executed by the Task statemachine." >> $actionSMDoc
echo " " >> $actionSMDoc
echo "Note: Note each firmware to be installed is one action state machine." >> $actionSMDoc
echo " " >> $actionSMDoc

echo '```mermaid' >> $actionSMDoc
./flasher export-statemachine --action >> $actionSMDoc
echo '```' >> $actionSMDoc


## generate state transition docs

./flasher export-statemachine --action --json > $actionSMJSON

echo "## Task Action (sub-statemachine) transitions" >> $actionSMDoc
echo " " >> $actionSMDoc

./docs/statemachine/generate_action_sm_docs.sh $actionSMJSON $actionSMDoc