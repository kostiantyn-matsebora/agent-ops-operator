# console-activity-replay Specification

## Purpose
Replaying the activity feed on the topology: a past window frame by frame, as a
mesh graph replays traffic, and one conversation hop by hop with its real gaps,
which a mesh cannot do because a request is gone once its trace is sampled.

## Requirements

### Requirement: A past window replays frame by frame

The topology SHALL offer a replay of a past window on any view. The operator
SHALL choose the interval from one, five, ten and thirty minutes, and the
window SHALL be divided into frames of ten seconds.

A slider SHALL select the frame, a play control SHALL advance it at one of
three speeds, and the view SHALL show the frame's time and index.

In a frame the graph SHALL show the rates and traffic of the minute ending at
that frame, and every hop of that frame SHALL pulse.

The replay SHALL be bounded by the activity buffer. A window reaching past
what the buffer holds SHALL be reported as such, not rendered as quiet.
Leaving the replay SHALL return to live without a reload.

#### Scenario: A frame shows its minute
- **WHEN** the slider is moved to a frame
- **THEN** every edge shows the rate of the minute ending at that frame, the hops of that ten seconds pulse, and the hop feed lists them

#### Scenario: Speed is chosen
- **WHEN** play is pressed at the fastest speed
- **THEN** frames advance once a second until the end, and pausing keeps the current frame

#### Scenario: The buffer's edge is admitted
- **WHEN** a thirty minute interval is chosen and the buffer holds twelve
- **THEN** the view says the first eighteen minutes are not held, rather than showing them as silent

### Requirement: One conversation replays hop by hop

The topology SHALL offer a replay of one conversation on any view.

The hops of its latest run SHALL be listed in order with their offsets from
the first hop and their latencies, and stepped one at a time, by a play
control, by a step control, or by choosing a hop.

The current hop SHALL pulse on the graph, every element off the conversation's
route SHALL be dimmed, and the panel SHALL show what the current hop carried.
Play SHALL compress the real gaps so a long gap reads as long without reading
as forever.

A conversation SHALL be replayable from its node, from its pod, from any of
its hops, and from a list of conversations active in the window, on any view.

#### Scenario: A slow run is attributable
- **WHEN** a conversation whose runtime took thirty seconds to start is replayed
- **THEN** the pod start hop shows its thirty second latency, and the step to the dispatch shows the same offset

#### Scenario: The route is the only thing lit
- **WHEN** a conversation is replayed on the Components view
- **THEN** only the manager, context-sync, its runtime image, egress-proxy, its model, its tools and its channels' adapters keep full contrast

#### Scenario: Leaving returns to live
- **WHEN** the conversation replay is closed
- **THEN** the view returns to live traffic with the display selections untouched
