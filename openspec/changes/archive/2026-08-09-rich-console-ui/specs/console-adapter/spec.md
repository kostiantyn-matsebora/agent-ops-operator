# console-adapter (delta)

## MODIFIED Requirements

### Requirement: Observed vs joined conversations
A conversation SHALL be **joined** when the console Channel holds a thread binding in `status.threads[]`, and **observed** otherwise. Observed conversations SHALL be fully visible — phase, runs, results, graph, sequence — but SHALL carry no composer, because there is no thread to post into.

Conversations the console **originated** SHALL arrive joined without any Pipeline edit: the router appends the originating channel, so a console-started conversation already holds a console thread. Joining a Pipeline SHALL therefore be required only to observe and reply to conversations that other signal sources started.

Where a conversation is observed and not joined, the UI SHALL state why and print the exact patch that would add the console Channel to that Pipeline's `channelRefs[]`. Neither the chart nor the console SHALL make that edit.

#### Scenario: Unjoined pipeline is watchable but not writable
- **WHEN** a Conversation belongs to a Pipeline whose channels do not include the console channel
- **THEN** the console shows its phase, runs, and results from CR status but offers no message input

#### Scenario: Self-started conversations are immediately replyable
- **WHEN** a user starts a conversation from the console against a pipeline that does not list the console channel
- **THEN** the conversation holds a console thread and its composer is live, with no Pipeline edit

#### Scenario: Another source's work is visible but read-only
- **WHEN** a conversation started by a cluster-event source belongs to a pipeline that does not list the console channel
- **THEN** it is fully visible, has no composer, and the UI shows the reason and the joining patch

#### Scenario: The console makes no wiring edits
- **WHEN** any unjoined pipeline is displayed
- **THEN** the console has written nothing to the Pipeline, and only prints the patch

## ADDED Requirements

### Requirement: The console holds a signal identity alongside its channel identity
The console SHALL serve both the channel adapter contract (carrying conversations) and the signal adapter contract (originating them), using two distinct derived tokens in one pod. Each token SHALL open only its own contract surface. The channel identity SHALL never be used to originate, and the signal identity SHALL never be used to reply.

#### Scenario: Identities do not substitute for one another
- **WHEN** the channel token is presented to `/signal/inbound`, or the signal token to `/channel/inbound`
- **THEN** the request is rejected

#### Scenario: One workload, two contracts
- **WHEN** the console is running
- **THEN** a single pod long-polls `/channel/ops` and posts to `/signal/inbound`, and no second workload exists for the signal role
