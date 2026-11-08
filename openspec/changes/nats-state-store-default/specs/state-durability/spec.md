## ADDED Requirements

### Requirement: A default install's state home is a clustered state server
Where the `nats` backend is selected, the state store home SHALL be the state
server bundle's own volumes, one per server. The manager SHALL still mount no
PersistentVolume and SHALL read and write the store only over the network.

A default install therefore has three homes, not two: the Kubernetes API for
configuration, the clustered state server for conversation state, and the
PersistentVolumes of the runtimes. The state server's loss of any one server
SHALL NOT lose a record.

#### Scenario: The manager still mounts nothing
- **WHEN** the manager's pod is rendered for the `nats` backend
- **THEN** it mounts no PersistentVolume

#### Scenario: One server's volume is lost
- **WHEN** one state server's volume is lost and the server rejoins empty
- **THEN** it is repopulated from the other two and no record is lost
