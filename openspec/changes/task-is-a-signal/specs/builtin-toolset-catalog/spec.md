## MODIFIED Requirements

### Requirement: A route can observe without executing
Binding only the observation toolset on a Pipeline SHALL yield conversations whose allowlist contains the observation tools and NOT the execution tools. Because a Pipeline's bindings ARE its conversations' capabilities — profiles contribute nothing, and nothing supplies a default — this needs no profile edit and no change of composition mode: a profile serving several routes grants shell on one and withholds it on another purely by what each Pipeline binds. This holds for every route uniformly, including a Pipeline reached by a posted task through a source it claims and one named by a chat command.

#### Scenario: One profile, two routes, different shell access
- **WHEN** two Pipelines route to one profile, one binding the observation toolset and one binding observation plus execution
- **THEN** conversations from the first cannot use `Bash` while conversations from the second can, and the AgentProfile is identical for both

#### Scenario: Withholding shell needs no profile edit
- **WHEN** an operator removes execution from a route by changing only that route's Pipeline binding
- **THEN** other Pipelines sharing the profile keep their previous capabilities

#### Scenario: A reached Pipeline governs what a task or command may do
- **WHEN** a Pipeline that binds observation only is reached by a `kind: task` signal on a source it claims, or by a chat command naming it
- **THEN** the resulting conversation can observe but not execute, exactly as for a signal-routed one
