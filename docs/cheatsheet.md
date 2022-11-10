# get flasher task attribute for device

```
flasher get task --device-id f0c8e4ac-5cce-4370-93ff-bd3196fd3b9e \
                 --config ./samples/flasher-client.yaml
```

# set device for firmware install - force to skip installed component version checks.
```
flasher set install-firmware --device-id fa1e8306-b8a6-425b-8f84-fb747d73d399 \
                              --inventory-source=serverservice \
                              --config ./samples/flasher-client.yaml \
                              --force
```

# delete firmware install task for device
```
flasher delete task --device-id f0c8e4ac-5cce-4370-93ff-bd3196fd3b9e \
                    --config ./samples/flasher-client.yaml
```

# run worker
```
flasher run worker --inventory-source=serverservice \
                   --config ./samples/flasher-worker.yaml \
                   --trace
```

# export flasher Task action sub-statemachine JSON representation doc
```
flasher export statemachine --action  > ./docs/statemachine/action-statemachine.json
```

# export flasher Task statemachine JSON representation doc
```
flasher export statemachine --task  > ./docs/statemachine/task-statemachine.json
```


