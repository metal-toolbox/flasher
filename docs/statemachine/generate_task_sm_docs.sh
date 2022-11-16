#!/bin/bash
# Generates state machine documentation for flasher.
#
# script adapted from https://github.com/omertuc/assisted-service/tree/statemachine/cmd/graphstatemachine
#
set -euxo pipefail

SCRIPT_DIR=$(dirname "$(readlink -f "$0")")

JSON="${SCRIPT_DIR}"/task-statemachine.json
OUT_FILE="${SCRIPT_DIR}"/README-task-statemachine.md

echo Compiling and running go state machine doc JSON dump...
#go run "${SCRIPT_DIR}"/main.go | jq >"${JSON}"
echo "${JSON}" generated

MEDIA="${SCRIPT_DIR}/media-task-sm"

mkdir -p "${MEDIA}"

function getStateName() {
	jq --arg state "$1" '.states[$state].name' -r "${JSON}"
}

function getStateDescription() {
	jq --arg state "$1" '.states[$state].description' -r "${JSON}"
}

function getTransitionTypeName() {
	jq --arg transition_type "$1" '.transition_types[$transition_type].name' -r "${JSON}"
}

function getTransitionTypeDescription() {
	jq --arg transition_type "$1" '.transition_types[$transition_type].description' -r "${JSON}"
}

function getSourceStateTransitionTypes() {
	jq --arg state "$1" '[.transition_rules[]
        | select(
            .source_states | index($state)
        ).transition_type] | sort | unique | .[]' -r "${JSON}"
}

function getSourceStateTransitionRules() {
	jq --arg state "$1" '[.transition_rules[]
        | select(
            .source_states | index($state)
        ).name] | sort | unique | .[]' -r "${JSON}"
}

function getDestinationStateTransitionTypes() {
	jq --arg state "$1" '[.transition_rules[]
        | select(
            .destination_state == $state
        ).transition_type] | sort | unique | .[]' -r "${JSON}"
}

function getDestinationStateTransitionRules() {
	jq --arg state "$1" '[.transition_rules[]
        | select(
            .destination_state == $state
        ).name] | sort | unique | .[]' -r "${JSON}"
}

function getTransitionTypeSourceStates() {
	jq --arg transition_type "$1" '[.transition_rules[]
        | select(
            .transition_type == $transition_type
        ).source_states[]] | sort | unique | .[]' -r "${JSON}"
}

function getTransitionTypeDestinationStates() {
	jq --arg transition_type "$1" '[.transition_rules[]
        | select(
            .transition_type == $transition_type
        ).destination_state] | sort | unique | .[]' -r "${JSON}"
}

function getTransitionTypeTransitionRules() {
	jq --arg transition_type "$1" '[.transition_rules[]
        | select(
            .transition_type == $transition_type
        ).name] | sort | unique | .[]' -r "${JSON}"
}

function github_markdown_linkify() {
	jq -n --arg name "$1" '$name
        | gsub(" "; "-")
        | gsub("[^a-zA-Z0-9-]"; "")
        | ascii_downcase
        | "[\($name)](#\(.))"
    ' -r
}

echo "# Flasher task state machine" >"${OUT_FILE}"

echo "The Task statemachine plans and executes Action (sub-statemachines) to install firmware." >>"${OUT_FILE}"
echo "" >>"${OUT_FILE}"

echo Generating table of contents...

echo "## Table of Contents" >>"${OUT_FILE}"
echo "" >>"${OUT_FILE}"
echo "### States" >>"${OUT_FILE}"
for state in $(jq '.states | keys[]' -r "${JSON}"); do
	echo "* $(github_markdown_linkify "$(getStateName $state)")" >>"${OUT_FILE}"
done
echo "" >>"${OUT_FILE}"
echo "### Transition Types" >>"${OUT_FILE}"
echo "Transition types are the events that can cause a state transition" >>"${OUT_FILE}"
echo "" >>"${OUT_FILE}"
for transition_type in $(jq '.transition_types | keys[]' -r "${JSON}"); do
	echo "* $(github_markdown_linkify "$(getTransitionTypeName "$transition_type")")" >>"${OUT_FILE}"
done
echo "" >>"${OUT_FILE}"
echo "### Transition Rules" >>"${OUT_FILE}"
echo "Transition rules are the rules that define the required source states and conditions needed to move to a particular destination state when a particular transition type happens" >>"${OUT_FILE}"
echo "" >>"${OUT_FILE}"
jq '.transition_rules[]' -c "${JSON}" | while read -r transition_rule_json; do
	transition_rule_name=$(echo "$transition_rule_json" | jq '.name' -r)
	echo "* $(github_markdown_linkify "$transition_rule_name")" >>"${OUT_FILE}"
done
echo "" >>"${OUT_FILE}"

echo "## States" >>"${OUT_FILE}"
for state in $(jq '.states | keys[]' -r "${JSON}"); do
	echo Processing "$state"
	state_name=$(getStateName "$state")
	state_description=$(getStateDescription "$state")

	echo "### $state_name" >>"${OUT_FILE}"
	echo "$state_description" >>"${OUT_FILE}"
	echo "" >>"${OUT_FILE}"

	echo "#### Transition types where this is the source state" >>"${OUT_FILE}"
	for transition_type in $(getSourceStateTransitionTypes "$state"); do
		echo "* $(github_markdown_linkify "$(getTransitionTypeName "$transition_type")")" >>"${OUT_FILE}"
	done

	echo "" >>"${OUT_FILE}"

	echo "#### Transition types where this is the destination state" >>"${OUT_FILE}"
	for transition_type in $(getDestinationStateTransitionTypes "$state"); do
		echo "* $(github_markdown_linkify "$(getTransitionTypeName "$transition_type")")" >>"${OUT_FILE}"
	done

	echo "" >>"${OUT_FILE}"

	echo "#### Transition rules where this is the source state" >>"${OUT_FILE}"
	jq --arg state "$state" '
    # Keep a copy of the root node for future use
    . as $root |

    # We only care about transitions for which our state is a source state
    [
        .transition_rules[]
        | .source_state = .source_states[]
        | select(.source_state == $state)
    ] as $relevant_rules |

    # Create the actual graph JSON
    {
        "states": [
            [
                $relevant_rules[]
                | .destination_state
            ] + [$state]
            | sort
            | unique[]
            | {
                "name": .,
                "type": "regular"
            }
        ],
        "transitions": [
            $relevant_rules[]
            | {
                "from": .source_state,
                "to": .destination_state,
                "label": "\($root.transition_types[.transition_type].name) - \(.name)",
            }
        ]
    }
    ' "${JSON}" |
		smcat \
			--input-type json \
			--output-type svg \
			--direction left-right \
			--engine dot \
            --dot-graph-attrs "bgcolor=transparent splines=line" \
			--output-to "${MEDIA}"/source_"${state}".svg

	echo "![source_${state}](./$(basename "${MEDIA}")/source_${state}.svg)" >>"${OUT_FILE}"
	echo "" >>"${OUT_FILE}"

	getSourceStateTransitionRules "$state" | while read -r transition_rule; do
		echo "* $(github_markdown_linkify "$transition_rule")" >>"${OUT_FILE}"
	done

	echo "" >>"${OUT_FILE}"

	echo "#### Transition rules where this is the destination state" >>"${OUT_FILE}"
	jq --arg state "$state" '
    # Keep a copy of the root node for future use
    . as $root |

    # We only care about transitions for which our state is the destination state
    [
        .transition_rules[]
        | select(.destination_state == $state)
    ] as $relevant_rules |

    # Create the actual graph JSON
    {
        "states": [
            [
                $relevant_rules[]
                | .source_states[]
            ] + [$state]
            | sort
            | unique[]
            | {
                "name": .,
                "type": "regular"
            }
        ],
        "transitions": [
            $relevant_rules[]
            | .source_state = .source_states[]
            | {
                "from": .source_state,
                "to": .destination_state,
                "label": "\($root.transition_types[.transition_type].name) - \(.name)",
            }
        ]
    }
    ' "${JSON}" |
		smcat \
			--input-type json \
			--output-type svg \
			--direction left-right \
			--engine dot \
            --dot-graph-attrs "bgcolor=transparent splines=line" \
			--output-to "${MEDIA}"/destination_"${state}".svg
	echo "![destination_${state}](./$(basename ${MEDIA})/destination_${state}.svg)" >>"${OUT_FILE}"

	echo "" >>"${OUT_FILE}"

	getDestinationStateTransitionRules "$state" | while read -r transition_rule; do
		echo Processing "$state" "$transition_rule"
		echo "* $(github_markdown_linkify "$transition_rule")" >>"${OUT_FILE}"
	done

	echo "" >>"${OUT_FILE}"
done
echo "" >>"${OUT_FILE}"

echo "## Transition Types" >>"${OUT_FILE}"
echo "Transition types are the events that can cause a state transition" >>"${OUT_FILE}"
echo "" >>"${OUT_FILE}"
for transition_type in $(jq '.transition_types | keys[]' -r "${JSON}"); do
	echo Processing "$transition_type"
	transition_type_name=$(getTransitionTypeName "$transition_type")
	transition_type_description=$(getTransitionTypeDescription "$transition_type")

	echo "### $transition_type_name" >>"${OUT_FILE}"
	echo "$transition_type_description" >>"${OUT_FILE}"
	echo "" >>"${OUT_FILE}"

	echo "#### Source states where this transition type applies" >>"${OUT_FILE}"
	for state in $(getTransitionTypeSourceStates "$transition_type"); do
		echo "* $(github_markdown_linkify "$(getStateName "$state")")" >>"${OUT_FILE}"
	done

	echo "" >>"${OUT_FILE}"

	echo "#### Destination states where this transition type applies" >>"${OUT_FILE}"
	for state in $(getTransitionTypeDestinationStates "$transition_type"); do
		echo "* $(github_markdown_linkify "$(getStateName "$state")")" >>"${OUT_FILE}"
	done

	echo "#### Transition rules using this transition type" >>"${OUT_FILE}"

	jq --arg transition_type "$transition_type" '
    # Keep a copy of the root node for future use
    . as $root |

    # We only care about transitions for which our state is the destination state
    [
        .transition_rules[]
        | select(.transition_type == $transition_type)
    ] as $relevant_rules |

    # Create the actual graph JSON
    {
        "states": [
            [
                $relevant_rules[]
                | (.source_states + [.destination_state])[]
            ]
            | sort
            | unique[]
            | {
                "name": .,
                "type": "regular"
            }
        ],
        "transitions": [
            $relevant_rules[]
            | .source_state = .source_states[]
            | {
                "from": .source_state,
                "to": .destination_state,
                "label": "\($root.transition_types[.transition_type].name) - \(.name)",
            }
        ]
    }
    ' "${JSON}" |
		smcat \
			--input-type json \
			--output-type svg \
			--direction left-right \
			--engine dot \
            --dot-graph-attrs "bgcolor=transparent splines=line" \
			--output-to "${MEDIA}"/transition_type_"${transition_type}".svg
	echo "![transition_type_${transition_type}](./$(basename "${MEDIA}")/transition_type_${transition_type}.svg)" >>"${OUT_FILE}"

	echo "" >>"${OUT_FILE}"

	getTransitionTypeTransitionRules "$transition_type" | while read -r transition_rule; do
		echo Processing "$state" "$transition_rule"
		echo "* $(github_markdown_linkify "$transition_rule")" >>"${OUT_FILE}"
	done
done
echo "" >>"${OUT_FILE}"

echo "## Transition Rules" >>"${OUT_FILE}"
echo "Transition rules are the rules that define the required source states and conditions needed to move to a particular destination state when a particular transition type happens" >>"${OUT_FILE}"
echo "" >>"${OUT_FILE}"
jq '.transition_rules[]' -c "${JSON}" | while read -r transition_rule_json; do
	transition_rule_name=$(echo "$transition_rule_json" | jq '.name' -r)
	echo Processing "$transition_rule_name"
	transition_rule_description=$(echo "$transition_rule_json" | jq '.description' -r)
	transition_rule_source_states=$(echo "$transition_rule_json" | jq '.source_states[]' -r)
	transition_rule_destination_state=$(echo "$transition_rule_json" | jq '.destination_state' -r)

	echo "### $transition_rule_name" >>"${OUT_FILE}"
	echo "$transition_rule_description" >>"${OUT_FILE}"
	echo "" >>"${OUT_FILE}"

	echo "#### Source states" >>"${OUT_FILE}"
	for state in $transition_rule_source_states; do
		echo "* $(github_markdown_linkify "$(getStateName "$state")")" >>"${OUT_FILE}"
	done

	echo "" >>"${OUT_FILE}"

	echo "#### Destination state" >>"${OUT_FILE}"
	echo "$(github_markdown_linkify "$(getStateName "$transition_rule_destination_state")")" >>"${OUT_FILE}"
	echo "" >>"${OUT_FILE}"
done

echo "" >>"${OUT_FILE}"
