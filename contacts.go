package meshcore

// Contact identifies a peer known to the device. Full contact synchronisation
// arrives in Phase 4; the type is defined here because events reference it.
type Contact struct {
	Name      string
	PublicKey string
}
