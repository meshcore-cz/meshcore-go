package cli

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

// shareTypeNames maps the numeric MeshCore advert type to its label, matching
// the type= parameter the MeshCore app expects in a meshcore:// contact link.
var shareTypeNames = map[int]string{
	1: "companion",
	2: "repeater",
	3: "room",
	4: "sensor",
}

// cmdShare prints a QR code and meshcore:// link for the connected device so
// others can scan it to add the device as a contact in the MeshCore app.
func cmdShare(ctx context.Context, e *env) error {
	name, pubKey, err := connectedIdentity(ctx, e)
	if err != nil {
		return err
	}
	pubKey = strings.ToLower(strings.TrimSpace(pubKey))
	if pubKey == "" {
		return fmt.Errorf("device reported no public key to share")
	}

	typ := 1
	if v := e.args.flag("type"); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || shareTypeNames[n] == "" {
			return fmt.Errorf("invalid --type %q (1=companion, 2=repeater, 3=room, 4=sensor)", v)
		}
		typ = n
	}

	shareName := strings.TrimSpace(name)
	if shareName == "" {
		shareName = "mc-" + pubKey[:6]
	}
	uri := contactShareURI(shareName, pubKey, typ)

	if e.out.JSON {
		return e.out.JSONValue(map[string]any{
			"uri":        uri,
			"name":       shareName,
			"public_key": pubKey,
			"type":       typ,
			"type_name":  shareTypeNames[typ],
		})
	}

	// A QR code is only useful on a terminal; when piped or suppressed, print
	// just the link.
	if !e.args.has("no-qr") && ui.IsTerminal(e.out.Out) {
		qr, qrErr := ui.RenderQR(uri, ui.ColorEnabled(e.out.Out))
		if qrErr != nil {
			return qrErr
		}
		e.out.Human("\n%s\n", qr)
	}
	e.out.Human("Scan to add %q in the MeshCore app:\n", shareName)
	e.out.Human("%s\n", uri)
	return nil
}

// contactShareURI builds the meshcore://contact/add link understood by the
// MeshCore mobile app. See https://docs.meshcore.io/qr_codes/.
func contactShareURI(name, pubKey string, typ int) string {
	q := url.Values{}
	q.Set("name", name)
	q.Set("public_key", pubKey)
	q.Set("type", strconv.Itoa(typ))
	return "meshcore://contact/add?" + q.Encode()
}

// connectedIdentity resolves the connected device's name and full public key,
// preferring the cached backend status snapshot over live radio I/O.
func connectedIdentity(ctx context.Context, e *env) (name, publicKey string, err error) {
	st, backendRunning := backendStatus(ctx, e)
	if backendRunning && st.Healthy && !e.args.has("direct") && st.Device.Available() && st.Device.PublicKey != "" {
		return st.Device.Name, st.Device.PublicKey, nil
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return "", "", err
	}
	defer backend.Close()

	info, err := backend.DeviceInfo(ctx)
	if err != nil {
		return "", "", err
	}
	return info.Name, info.PublicKey, nil
}
