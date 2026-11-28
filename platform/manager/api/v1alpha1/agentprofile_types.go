package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OutputFormat is an agent's declared output contract. See
// AgentProfileSpec.OutputFormat — required, and with no default on purpose.
type OutputFormat string

const (
	// OutputFormatBlocks appends the shared output-format specification.
	OutputFormatBlocks OutputFormat = "blocks"
	// OutputFormatNone appends nothing; the profile's prompt owns formatting.
	OutputFormatNone OutputFormat = "none"
)

// RepoAuthType selects the git auth mechanism.
// +kubebuilder:validation:Enum=ssh;https
type RepoAuthType string

const (
	RepoAuthSSH   RepoAuthType = "ssh"
	RepoAuthHTTPS RepoAuthType = "https"
)

// RepoAuth references credentials for a (possibly private) git repository.
type RepoAuth struct {
	Type RepoAuthType `json:"type"`
	// SecretRef points to a Secret holding either key `sshKey` (type=ssh,
	// private deploy key) or key `token` (type=https, PAT; optional `username`).
	SecretRef ObjectRef `json:"secretRef"`
}

// RepositorySpec identifies the git repository an agent runs from. The runtime
// checks it out as its working directory, so the agent has access to the whole
// repo (CLAUDE.md, .claude/agents, skills, any assets). Optional: without a
// repository the agent runs as a pure advisor over its tools.
//
// Deprecated: superseded by RepositoryEntry / `repositories[]`
// (multi-repo-workspace). This stays as `AgentProfileSpec.Repository`'s
// deprecated one-entry shorthand — not slated for removal.
type RepositorySpec struct {
	// URL of the repository to check out, in any form git understands
	// (ssh://, git@host:path, https://). Empty means no checkout.
	// +optional
	URL string `json:"url,omitempty"`
	// Branch or ref to check out.
	// +kubebuilder:default=main
	// +optional
	Ref string `json:"ref,omitempty"`
	// +optional
	Auth *RepoAuth `json:"auth,omitempty"`
}

// MaxRepoName bounds RepositoryEntry.Name, a DNS label.
const MaxRepoName = 63

// RepositoryEntry is one repository in a `repositories[]` list
// (multi-repo-workspace), replacing the deprecated single-repository
// RepositorySpec. A `repositories[]` list on one holder REPLACES another's,
// never merging — see the "capability's list replaces the profile's list"
// requirement.
type RepositoryEntry struct {
	// Name is this entry's identity within the list — a DNS label, and the
	// list's map key, so the API server itself refuses two entries sharing
	// one name.
	//
	// INERT for a single repository: there is no `/workspace/<name>` alias
	// for it. It exists to be addressed once a SECOND repository joins the
	// list — by `agentFile.repo`, `workingDir`, or a `<repo>:<path>` prompt
	// qualifier.
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`
	// URL of the repository to check out, in any form git understands
	// (ssh://, git@host:path, https://).
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`
	// Ref: branch, tag or commit to check out.
	// +kubebuilder:default=main
	// +optional
	Ref string `json:"ref,omitempty"`
	// +optional
	Auth *RepoAuth `json:"auth,omitempty"`
	// KeepGit keeps the `.git` directory after the clone. Absent means
	// TRUE — the agent can inspect history and the tree still looks like
	// an ordinary checkout. False has the workspace-sync init container
	// remove `.git` once the clone completes.
	// +kubebuilder:default=true
	// +optional
	KeepGit *bool `json:"keepGit,omitempty"`
}

// KeepsGit reports whether e keeps its `.git` directory after the clone.
// Absent (nil) means true — see KeepGit's own comment.
func (e RepositoryEntry) KeepsGit() bool {
	return e.KeepGit == nil || *e.KeepGit
}

// AgentFileRef names an agent definition file by its location, instead of
// the conventional `agent` name form (multi-repo-workspace). Mutually
// exclusive with `agent` on a profile — see AgentProfileSpec's own CEL rule.
//
// +kubebuilder:validation:XValidation:rule="!self.path.startsWith('/') && !self.path.contains('..')",message="agentFile.path must be relative to the repository root and must not contain '..'"
type AgentFileRef struct {
	// Repo names the `repositories[]` entry holding the file. Optional
	// when exactly one repository resolves, required with two or more —
	// enforced on the holder's Ready condition (multi-repo-workspace),
	// never here: admission sees only this object, never the resolved
	// list a Pipeline or Coordinator reaches it through.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	Repo string `json:"repo,omitempty"`
	// Path to the file, relative to Repo's root. Never absolute, never
	// containing `..`. A path that passes this check yet resolves outside
	// the repository through a symlink is refused at pod start instead,
	// naming the path — admission cannot see the checked-out tree.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Path string `json:"path"`
}

// AgentProfileSpec is an addressable agent IDENTITY: repository, agent role,
// prompts, credentials and limits. It carries NO capabilities — what an agent
// may DO (its tool allowlist and MCP servers) comes exclusively from the
// Pipeline routing its conversation, so one profile serves routes with
// genuinely different capabilities without being cloned or edited.
//
// +kubebuilder:validation:XValidation:rule="!(has(self.repository) && has(self.repositories))",message="repository and repositories are mutually exclusive: repository is the deprecated one-entry shorthand repositories replaces"
// +kubebuilder:validation:XValidation:rule="!(has(self.agent) && has(self.agentFile))",message="agent and agentFile are mutually exclusive: agent is the conventional single-repository name form, agentFile names a file by location"
type AgentProfileSpec struct {
	// Repository is the single-repository shorthand for Repositories below
	// — resolving as a one-entry list named `default`, inert since there
	// is only one entry (multi-repo-workspace).
	//
	// Deprecated: use Repositories. Setting both on one profile is refused
	// — kept working indefinitely as shorthand, unlike RuntimeRef this is
	// not slated for removal.
	//
	// A POINTER, not a value: `has(self.repository)` in the CEL rule above
	// must see this as absent until a caller sets it. A non-pointer struct
	// is never empty to `encoding/json`'s `omitempty`, so every profile
	// written through a typed client would round-trip `repository: {}` and
	// trip the exclusivity rule against any `repositories[]` it also set.
	// +optional
	Repository *RepositorySpec `json:"repository,omitempty"`

	// Repositories lists every repository this agent's workspace checks
	// out. Mutually exclusive with Repository — setting both is refused.
	// See multi-repo-workspace for resolution (a capability's list
	// replaces a profile's, never merging), the snapshot onto the
	// Conversation, and the workspace layout the count produces.
	// +listType=map
	// +listMapKey=name
	// +optional
	Repositories []RepositoryEntry `json:"repositories,omitempty"`

	// AgentFile names the agent definition file by its location, instead
	// of the conventional Agent name form below — mutually exclusive with
	// it. Travels with Repositories: a capability that replaces the list
	// also replaces (or clears) this.
	// +optional
	AgentFile *AgentFileRef `json:"agentFile,omitempty"`

	// WorkingDir names a Repositories entry the agent starts in, instead
	// of the workspace root. Valid only once the resolved list holds two
	// or more repositories — enforced on the holder's (Pipeline or
	// Coordinator) Ready condition, not here: admission sees only this
	// profile, never the resolved list.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	WorkingDir string `json:"workingDir,omitempty"`

	// Agent is the agent to adopt: name of `.claude/agents/<agent>.md` in the
	// repository. A profile names one agent and a Pipeline names one profile,
	// so the agent comes from the wiring and no message may select another.
	// Mutually exclusive with AgentFile.
	// +optional
	Agent string `json:"agent,omitempty"`

	// Prompt / ReplyPrompt are repo-relative template paths (job-style lanes).
	// When empty, the operator's built-in lane templates wrap the agent.
	// +optional
	Prompt string `json:"prompt,omitempty"`
	// ReplyPrompt wraps a follow-up in an existing conversation, where Prompt
	// wraps the first unit.
	// +optional
	ReplyPrompt string `json:"replyPrompt,omitempty"`

	// SystemPrompt is INLINE role text appended to the agent's system prompt,
	// for profiles with no repository — where `agent` can name no
	// `.claude/agents/<agent>.md` because nothing is checked out. It is
	// identity, not capability: it shapes how the agent behaves, never what it
	// may call (that is the Pipeline's toolsets, always).
	//
	// Appended, never replacing: the runtime keeps its own system prompt and
	// adds this. A profile WITH a repository should carry its role in the
	// definition file instead, which is version-controlled and can declare
	// tools; this exists so a repo-less profile is not silently personality-free.
	// +optional
	SystemPrompt string `json:"systemPrompt,omitempty"`

	// OutputFormat declares this agent's OUTPUT CONTRACT. REQUIRED, and
	// deliberately without a default, because both candidate defaults are wrong:
	//
	//   blocks — the operator's shared output-format specification is appended
	//            to the prompt: the block grammar, the fold, the markdown subset
	//            and a default section set
	//   none   — NOTHING is appended, and the profile's own prompt owns
	//            formatting entirely
	//
	// NO DEFAULT ON PURPOSE. `none` leaves output unformatted unless the author
	// wrote a format into the prompt; `blocks` shapes output by something the
	// author never asked for. Refusing to guess is the honest resolution, so the
	// author declares it and a profile omitting the field is REFUSED.
	//
	// IDENTITY, NEVER CAPABILITY. It shapes how the agent SPEAKS, not what it
	// may call — the allowlist and the MCP servers remain exclusively the
	// originating Pipeline's.
	//
	// IT GATES THE PROMPT, NEVER THE PARSE. Adapters parse whatever they are
	// given, so a profile declaring `none` whose agent emits tags anyway is
	// still rendered as blocks. Decoupling them is what keeps this safe: a
	// switch that moved the parser too could be configured into a state where
	// the model emits tags nothing is looking for.
	//
	// IT IS ALSO THE COMPATIBILITY BOUNDARY. Nothing on the wire signals the
	// grammar — adapters parse the body, and no contract version changed — so an
	// adapter with no parser renders `<title>` as literal characters. What
	// protects it is a profile declaring `none`. Making the declaration
	// mandatory is what stops that protection being accidental.
	//
	// IT DOES NOT GATE THE OPERATOR'S OWN PROMPT CONTENT. Text stating that the
	// printed answer IS the deliverable is a fact about the system rather than a
	// preference, and is injected whatever this says.
	//
	// +kubebuilder:validation:Enum=blocks;none
	OutputFormat OutputFormat `json:"outputFormat"`

	// RuntimeRef selects the AgentRuntime (execution backend) for this
	// profile — deprecated, use `Pipeline.spec.runtimeRef`.
	//
	// Deprecated: moved to `Pipeline.spec.runtimeRef` and REMOVED IN THE NEXT
	// CHART MAJOR. It is read for one release only, below the Pipeline's own
	// field, so a profile applied before the upgrade keeps dispatching to the
	// runtime it named — the same posture the retired `sessionId` got.
	//
	// It moved because an AgentRuntime carries the ServiceAccount an agent runs
	// as, so a profile choosing one chose the agent's power in the cluster.
	// That made profile-edit rights into service-account-choice rights, while a
	// profile is prompts and a repo ref and a Pipeline already grants tools.
	// Whoever is trusted to grant capabilities is more qualified to choose an
	// execution identity, not less.
	//
	// An install setting this moves the ref to every Pipeline routing to this
	// profile; setting both is harmless, and the Pipeline wins.
	// +optional
	RuntimeRef *ObjectRef `json:"runtimeRef,omitempty"`

	// Env: extra environment for the agent process; values may use valueFrom
	// (secretKeyRef / configMapKeyRef) for credentials the agent needs. This
	// stays on the profile deliberately: these are the AGENT's own credentials
	// (an API token it was built around), not the route's capabilities, and
	// moving them would put secret references into the wiring object.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// MaxTurns bounds the agent's own turns within ONE work unit. It is a
	// runaway bound, not a budget: the conversation is unaffected.
	// +kubebuilder:default=60
	// +optional
	MaxTurns int32 `json:"maxTurns,omitempty"`

	// Resources for the runtime pod running this profile.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// AgentProfileStatus reports validation/resolution state.
type AgentProfileStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration of the last processed spec.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aprof
// +kubebuilder:printcolumn:name="Repo",type=string,JSONPath=`.spec.repository.url`
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.agent`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentProfile is an addressable agent definition (repo + role + MCP + env).
type AgentProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentProfileSpec   `json:"spec,omitempty"`
	Status AgentProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentProfileList contains a list of AgentProfile.
type AgentProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentProfile{}, &AgentProfileList{})
}
