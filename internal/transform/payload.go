package transform

// Payload is a destination-ready secret value and content type.
type Payload struct {
	ContentType string
	Value       []byte
}
