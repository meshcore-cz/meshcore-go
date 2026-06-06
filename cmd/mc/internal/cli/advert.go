package cli

import (
	"context"
)

// cmdAdvert implements `mc advert`: broadcast the device's own advertisement.
// The default is a zero-hop advert (neighbours only); --flood propagates it
// across the mesh.
func cmdAdvert(ctx context.Context, e *env) error {
	flood := e.args.has("flood")

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	if err := backend.Advertise(ctx, flood); err != nil {
		return err
	}

	mode := "zero-hop"
	if flood {
		mode = "flood"
	}
	e.out.Human("Advert sent (%s).\n", mode)
	return e.out.JSONValue(map[string]any{"sent": true, "flood": flood, "mode": mode})
}
