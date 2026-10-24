## Purpose
The synthesizer owns the synthesis contract; vendor implementations are adapters that poll it, choose a voice by language, and return audio in the format asked for.

## ADDED Requirements

### Requirement: Synthesis is a job an adapter claims

The synthesizer SHALL queue one job per text to speak, carrying `text`,
`lang` and the requested `format`, and SHALL serve `GET /speak/jobs` as a
long-poll adapters claim from with the same window, retry and at-least-once
rules as recognition. An adapter SHALL post `POST /speak/jobs/{id}/result`
with the audio. No adapter SHALL expose a port.

#### Scenario: A voice for the language
- **WHEN** a job carries `lang: de`
- **THEN** the adapter speaks it in a German voice, and the result names the voice used

#### Scenario: A format the adapter cannot produce
- **WHEN** a job asks for a format the adapter did not declare
- **THEN** the synthesizer does not offer it that job, and reports the gap when no adapter fits

### Requirement: An adapter declares its voices and formats

On each claim an adapter SHALL declare the languages it can speak and the
formats it can produce. The synthesizer SHALL route by both.

#### Scenario: The stub speaks silence
- **WHEN** the stub adapter is the only one polling
- **THEN** every job completes with a silent clip of the requested format, and the pipeline is testable end to end without a vendor

### Requirement: Vendors are adapters holding their own credential

Each vendor SHALL be its own adapter component with its credential projected
into its pod; the synthesizer SHALL hold none; tokens are derived with context
`synthesizer-adapter:<name>`.

#### Scenario: Swapping the vendor
- **WHEN** the Google adapter is replaced by a piper adapter
- **THEN** the synthesizer, the sender and every surface are unchanged
