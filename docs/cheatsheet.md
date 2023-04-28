# run worker
```
flasher run worker --store=serverservice \
                   --config ./samples/flasher-worker.yaml \
                   --trace
```

# export flasher Task action sub-statemachine [JSON|dot] representation doc
```
flasher export statemachine --action [--json]  > ./docs/statemachine/action-statemachine.json
```

# export flasher Task statemachine [JSON|dot] representation doc
```
flasher export statemachine --task [--json] > ./docs/statemachine/task-statemachine.json
```


