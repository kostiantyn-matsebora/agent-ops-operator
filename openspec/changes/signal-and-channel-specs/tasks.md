# Tasks: signal-and-channel-specs

## 1. API types + generic validation package

- [x] 1.1 Add `CredentialKeyDoc {Key, Required, Description}` and **spec** fields `ConfigSchema *runtime.RawExtension` (`+kubebuilder:pruning:PreserveUnknownFields`) + `CredentialKeys []CredentialKeyDoc` to `ChannelAdapterSpec` (`api/v1alpha1/channeladapter_types.go`) and `SignalAdapterSpec` (`api/v1alpha1/signaladapter_types.go`); add condition type + reason constants: `SchemaValid`/`InvalidSchema` (adapter CRs) and `ConfigValid`/`SchemaValidated`/`SchemaViolation` (Channel/SignalSource) where the other condition constants live
- [x] 1.2 Regenerate deepcopy and CRDs (`controller-gen object` + `crd` → `chart/files/crds/`) and verify the new spec fields render in `agentops.dev_channeladapters.yaml` / `agentops.dev_signaladapters.yaml`
- [x] 1.3 Create `internal/configschema/` wrapping `github.com/santhosh-tekuri/jsonschema/v6` (root module only): `Compile(raw []byte) (Schema, error)` and `Validate(schema, config []byte) []Violation` (nil config → `{}`; violations carry path + message, joined/truncated for condition messages); unit tests for compile failure, pass, required-field violation, nil config

## 2. Adapter CR reconcilers: schema compile check

- [x] 2.1 In `internal/controller/channeladapter_controller.go`: when `spec.configSchema` is set, compile it and set `SchemaValid` (True, or False/`InvalidSchema` with the compile error); confirm `Deployed`/`Ready` and workload rendering are untouched by an invalid schema
- [x] 2.2 Mirror in `internal/controller/signaladapter_controller.go`
- [x] 2.3 Integration (envtest) tests: valid schema → `SchemaValid=True`; garbage schema → `SchemaValid=False` + Deployment still created; no schema → no `SchemaValid` condition

## 3. Channel/SignalSource validation (`ConfigValid`)

- [x] 3.1 In `internal/controller/channel_controller.go`: after the `Served` logic, Get the `ChannelAdapter` named `spec.type`; if it declares a compilable `spec.configSchema`, validate `spec.config` via `internal/configschema` and set `ConfigValid` (True/`SchemaValidated`, False/`SchemaViolation` + violation message); remove the condition when no usable schema is declared; confirm the existing ChannelAdapter watch propagates spec changes (it does — no predicate filtering)
- [x] 3.2 Mirror in `internal/controller/signalsource_controller.go` against `SignalAdapter`; verify `Served`/`Wired` and ingestion are untouched by `ConfigValid=False`
- [x] 3.3 Integration (envtest) tests: schema declared → existing Channel/SignalSource re-reconciled and gains `ConfigValid`; violation → False with field named, `Served` unaffected; no declaration → condition absent; declaration removed / adapter CR deleted → condition cleared; uncompilable schema → no `ConfigValid` on served CRs

## 4. Reference declarations (no adapter code changes)

- [x] 4.1 Add the telegram declaration to the chart's gated `ChannelAdapter` template (`chart/templates/telegram-adapter.yaml`): configSchema (chatId string required; feedThreadId integer; approvers integer array; pollingEnabled boolean) + `credentialKeys: [{key: botToken, required: false}]`, beside the image ref
- [x] 4.2 Add the cron declaration to the `SignalAdapter` sample in `config/samples/` (schedule + input required, title optional); leave the vmalertmanager SignalAdapter (vm-bundle) without a declaration; verify `channel-telegram/`, `signal-cron/`, `signal-vmalertmanager/` modules have zero diffs

## 5. Verification + docs

- [x] 5.1 `go build ./... && go vet ./...`; full envtest suite (`KUBEBUILDER_ASSETS=... go test ./...`); `helm template` renders the telegram declaration when `telegramAdapter.enabled=true`
- [x] 5.2 Update README.md (config schema declaration on adapter CRs: discoverability straight from the CRD, advisory `ConfigValid`, `SchemaValid` compile check, authoring rule "bump schema with image") and CLAUDE.md (invariant nuance: adapter CRs carry no config *values*/connectivity/credentials — interface metadata like configSchema/credentialKeys is implementation declaration; manager never *interprets* config, mechanical adapter-declared-schema validation is allowed)
