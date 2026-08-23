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

// AgentProfileSpec is an addressable agent IDENTITY: repository, agent role,
// prompts, credentials and limits. It carries NO capabilities — what an agent
// may DO (its tool allowlist and MCP servers) comes exclusively from the
// Pipeline routing its conversation, so one profile serves routes with
// genuinely different capabilities without being cloned or edited.
type AgentProfileSpec struct {
	// +optional
	Repository RepositorySpec `json:"repository,omitempty"`

	// Agent is the agent to adopt: name of `.claude/agents/<agent>.md` in the
	// repository. A profile names one agent and a Pipeline names one profile,
	// so the agent comes from the wiring and no message may select another.
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
	// IT DOES NOT GATE THE OPERATOR'S OWN PROMPT CONTENT. Text stating that the
	// printed answer IS the deliverable is a fact about the system rather than a
	// preference, and is injected whatever this says.
	//
	// +kubebuilder:validation:Enum=blocks;none
	OutputFormat OutputFormat `json:"outputFormat"`

	// RuntimeRef selects the AgentRuntime (execution backend) for this
	// profile. Falls back to the AgentRuntime named "default", then to the
	// manager's bootstrap configuration.
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
