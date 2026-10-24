package probe

const KnownAdapterModel = "Vgate iCar Pro BLE 4.0 Dual"

const modelEvidence = "https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6383158184"

// DeviceInfo is contextual metadata, not an identification inferred from ATI.
// A caller with no model information must leave the model unknown.
type DeviceInfo struct {
	Model       string `json:"model"`
	ModelSource string `json:"model_source"`
	EvidenceURL string `json:"evidence_url,omitzero"`
}

func deviceInfo(model string) DeviceInfo {
	if model == "" {
		return DeviceInfo{ModelSource: "unknown"}
	}
	if model == KnownAdapterModel {
		return DeviceInfo{Model: model, ModelSource: "owner_reported", EvidenceURL: modelEvidence}
	}
	return DeviceInfo{Model: model, ModelSource: "user_supplied"}
}
