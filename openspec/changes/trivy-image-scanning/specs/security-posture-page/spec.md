## MODIFIED Requirements

### Requirement: The supply-chain answer is stated as it is, and no further

The page SHALL state what the published artifacts actually carry and SHALL name
what they do not. Images carrying provenance and an SBOM SHALL be stated together
with the absence of signatures and of chart attestation.

**Scanning is part of that answer, stated with its bound.** The page SHALL state
that every image is scanned for known vulnerabilities on the pull request that
builds it and on a schedule afterwards, that a fixable CRITICAL or HIGH finding
blocks the merge, and that a finding with no available fix is knowingly not
gated. Stating "scanned" without the bound would promise a clean image, which is
not what a gate on fixable findings delivers.

It SHALL NOT be given a section of its own while the answer is partial: a heading
promises a subject is handled, and this one is handled in part.

#### Scenario: A reviewer asks whether artifacts are verifiable

- **WHEN** the reader looks for supply-chain provenance
- **THEN** what images carry is stated, with the command that reads it
- **AND** the absence of signing and of chart attestation is stated in the same
  place, not omitted

#### Scenario: A reviewer asks whether the images are scanned

- **WHEN** the reader looks for vulnerability scanning
- **THEN** the page states when images are scanned and what blocks
- **AND** states in the same place that unfixable findings do not block
