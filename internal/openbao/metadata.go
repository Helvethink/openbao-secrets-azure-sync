package openbao

// Metadata is the subset of KV v2 metadata used by the controller.
type Metadata struct {
	CurrentVersion int
	Custom         map[string]string
}
